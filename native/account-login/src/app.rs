use std::io::BufRead;
use std::sync::{
    Arc,
    atomic::{AtomicBool, AtomicU64, Ordering},
};
use std::time::{Duration, Instant, SystemTime, UNIX_EPOCH};

use tao::event::{Event, WindowEvent};
use tao::event_loop::{ControlFlow, EventLoopBuilder, EventLoopProxy};
use tao::platform::run_return::EventLoopExtRunReturn;
use tao::window::WindowBuilder;
use wry::{PageLoadEvent, WebView, WebViewBuilder};

use crate::protocol::{self, Command, CommandKind, Failure, Outcome, Request};
use crate::session::{self, Metadata, Session, SessionCookie};

#[cfg(not(feature = "fixture"))]
const START_URL: &str = "https://accounts.google.com/ServiceLogin?service=youtube&continue=https%3A%2F%2Fmusic.youtube.com%2F";

#[cfg(not(feature = "fixture"))]
const METADATA: &str = r#"(() => {
  if (location.origin !== 'https://music.youtube.com') return null;
  const cfg = window.ytcfg;
  if (!cfg || typeof cfg.get !== 'function') return null;
  const raw = cfg.get('SESSION_INDEX');
  let index;
  if (typeof raw === 'number' && Number.isInteger(raw)) index = raw;
  else if (typeof raw === 'string' && /^(0|[1-9][0-9]?)$/.test(raw)) index = Number(raw);
  else return null;
  if (index < 0 || index > 99) return null;
  const delegated = cfg.get('DELEGATED_SESSION_ID');
  if (delegated !== undefined && delegated !== null && delegated !== '' &&
      (typeof delegated !== 'string' || !/^[A-Za-z0-9_-]{1,128}$/.test(delegated))) return null;
  return {accountIndex:index, onBehalfOfUser:delegated || ''};
})()"#;

#[cfg(feature = "fixture")]
const METADATA: &str =
    "window.fixtureConfirmed ? {accountIndex:3,onBehalfOfUser:'fixture_channel'} : null";

#[cfg(feature = "fixture")]
const FIXTURE_HTML: &str = r#"<!doctype html><html><head><meta charset="utf-8"><title>Synthetic account login</title></head><body style="font:18px system-ui;padding:24px"><h1>Synthetic account login</h1><p>Offline lifecycle test. No Google account or network is used.</p><button onclick="window.fixtureConfirmed=true">Complete synthetic login</button></body></html>"#;

// No Debug implementation: callbacks may carry account metadata.
enum Signal {
    Focus,
    Cancel,
    InputFailed,
    Navigating,
    Loaded {
        music: bool,
    },
    #[cfg(feature = "fixture")]
    FixtureComplete,
    Metadata {
        id: u64,
        raw: String,
    },
}

enum Stage {
    Before,
    After {
        metadata: Metadata,
        cookies: Vec<SessionCookie>,
    },
}

struct Probe {
    id: u64,
    generation: u64,
    started: Instant,
    stage: Stage,
}

fn start_input(mut input: impl BufRead + Send + 'static, proxy: EventLoopProxy<Signal>) -> bool {
    std::thread::Builder::new()
        .name("login-control".into())
        .spawn(move || {
            loop {
                let signal = match protocol::read_line(&mut input) {
                    Ok(Some(line)) => {
                        match serde_json::from_slice::<Command>(&line).map(|c| c.kind) {
                            Ok(CommandKind::Focus) => Signal::Focus,
                            Ok(CommandKind::Cancel) => Signal::Cancel,
                            #[cfg(feature = "fixture")]
                            Ok(CommandKind::FixtureComplete) => Signal::FixtureComplete,
                            Err(_) => Signal::InputFailed,
                        }
                    }
                    Ok(None) => Signal::Cancel,
                    Err(_) => Signal::InputFailed,
                };
                let stop = matches!(signal, Signal::Cancel | Signal::InputFailed);
                if proxy.send_event(signal).is_err() || stop {
                    return;
                }
            }
        })
        .is_ok()
}

#[cfg(any(not(feature = "fixture"), test))]
fn music_url(address: &str) -> bool {
    url::Url::parse(address).is_ok_and(|u| {
        u.scheme() == "https"
            && u.host_str() == Some("music.youtube.com")
            && u.port_or_known_default() == Some(443)
            && u.username().is_empty()
            && u.password().is_none()
    })
}

fn query(view: &WebView, proxy: &EventLoopProxy<Signal>, id: u64) -> bool {
    let proxy = proxy.clone();
    view.evaluate_script_with_callback(METADATA, move |raw| {
        // Only the fixed metadata object is accepted. Never transport page source.
        if raw.len() <= 1024 {
            let _ = proxy.send_event(Signal::Metadata { id, raw });
        } else {
            let _ = proxy.send_event(Signal::InputFailed);
        }
    })
    .is_ok()
}

#[cfg(any(windows, test))]
fn supported_windows_version(version: &str) -> bool {
    let pieces: Vec<_> = version.split('.').collect();
    if pieces.len() != 4 {
        return false;
    }
    let mut found = [0u32; 4];
    for (i, s) in pieces.into_iter().enumerate() {
        let Ok(n) = s.parse() else { return false };
        found[i] = n;
    }
    found >= [101, 0, 1210, 39]
}

#[cfg(any(target_os = "linux", test))]
fn supported_linux_version(version: &str) -> bool {
    let pieces: Vec<_> = version.split('.').collect();
    if pieces.len() != 3 {
        return false;
    }
    let mut found = [0u32; 3];
    for (i, s) in pieces.into_iter().enumerate() {
        let Ok(n) = s.parse() else { return false };
        found[i] = n;
    }
    found >= [2, 42, 0]
}

fn runtime_check() -> Result<(), Failure> {
    #[cfg(windows)]
    {
        let version = wry::webview_version().map_err(|_| Failure::RuntimeMissing)?;
        if !supported_windows_version(&version) {
            return Err(Failure::RuntimeOutdated);
        }
    }
    #[cfg(target_os = "linux")]
    {
        // System package dependencies must exist before the dynamic loader starts us.
        let version = wry::webview_version().map_err(|_| Failure::RuntimeMissing)?;
        if !supported_linux_version(&version) {
            return Err(Failure::RuntimeOutdated);
        }
    }
    Ok(())
}

pub fn run(request: Request, input: impl BufRead + Send + 'static) -> Outcome {
    if let Err(failure) = runtime_check() {
        return Outcome::error(failure);
    }
    let mut event_loop = EventLoopBuilder::<Signal>::with_user_event().build();
    #[cfg(target_os = "macos")]
    crate::macos_menu::install();
    let window = match WindowBuilder::new()
        .with_title(request.title)
        .with_inner_size(tao::dpi::LogicalSize::new(520.0, 740.0))
        .build(&event_loop)
    {
        Ok(window) => window,
        Err(_) => return Outcome::error(Failure::LoginFailed),
    };
    let proxy = event_loop.create_proxy();
    let loaded = proxy.clone();
    let navigating = proxy.clone();
    // Update these in native callbacks, before queued event-loop messages run.
    // A late metadata callback must not win a race with a navigation already begun.
    let generation = Arc::new(AtomicU64::new(0));
    let loading = Arc::new(AtomicBool::new(true));
    let navigation_generation = generation.clone();
    let navigation_loading = loading.clone();
    let load_generation = generation.clone();
    let load_loading = loading.clone();
    #[cfg(windows)]
    let mut profile = match crate::windows_profile::Profile::new() {
        Ok(profile) => profile,
        Err(_) => return Outcome::error(Failure::LoginFailed),
    };
    #[cfg(windows)]
    let mut context = wry::WebContext::new(Some(profile.path()));
    #[cfg(windows)]
    let builder = WebViewBuilder::new_with_web_context(&mut context);
    #[cfg(not(windows))]
    let builder = WebViewBuilder::new();
    let builder = builder
        .with_incognito(true)
        .with_navigation_handler(move |address| {
            #[cfg(feature = "fixture")]
            let allowed = address == "about:blank";
            #[cfg(not(feature = "fixture"))]
            let allowed = address.starts_with("https://");
            if allowed {
                navigation_generation.fetch_add(1, Ordering::SeqCst);
                navigation_loading.store(true, Ordering::SeqCst);
                let _ = navigating.send_event(Signal::Navigating);
            }
            allowed
        })
        .with_new_window_req_handler(|_, _| wry::NewWindowResponse::Deny)
        .with_download_started_handler(|_, _| false)
        .with_on_page_load_handler(move |event, address| {
            if matches!(event, PageLoadEvent::Started) {
                load_generation.fetch_add(1, Ordering::SeqCst);
                load_loading.store(true, Ordering::SeqCst);
                let _ = loaded.send_event(Signal::Navigating);
            }
            if matches!(event, PageLoadEvent::Finished) {
                #[cfg(feature = "fixture")]
                let music = address == "about:blank" || address.is_empty();
                #[cfg(not(feature = "fixture"))]
                let music = music_url(&address);
                load_loading.store(false, Ordering::SeqCst);
                let _ = loaded.send_event(Signal::Loaded { music });
            }
        });
    #[cfg(not(feature = "fixture"))]
    let builder = builder.with_url(START_URL);
    #[cfg(feature = "fixture")]
    let builder = builder.with_html(FIXTURE_HTML);

    #[cfg(not(target_os = "linux"))]
    let view = builder.build(&window);
    #[cfg(target_os = "linux")]
    let view = {
        use tao::platform::unix::WindowExtUnix;
        use wry::WebViewBuilderExtUnix;
        match window.default_vbox() {
            Some(container) => builder.build_gtk(container),
            None => return Outcome::error(Failure::LoginFailed),
        }
    };
    let view = match view {
        Ok(view) => view,
        Err(_) => return Outcome::error(Failure::LoginFailed),
    };
    #[cfg(feature = "fixture")]
    {
        // A previous helper must not have left even a persistent synthetic cookie.
        // This is this new private WebView's store, never a user's browser store.
        if !view.cookies().is_ok_and(|cookies| cookies.is_empty()) {
            drop(view);
            return Outcome::error(Failure::LoginFailed);
        }
        let cookie = wry::cookie::Cookie::build(("SAPISID", "synthetic-fixture-session"))
            .domain(".youtube.com")
            .path("/")
            .secure(true)
            .http_only(true)
            .expires(
                wry::cookie::time::OffsetDateTime::now_utc() + wry::cookie::time::Duration::days(1),
            )
            .build();
        if view.set_cookie(&cookie).is_err() {
            drop(view);
            return Outcome::error(Failure::LoginFailed);
        }
    }
    if !start_input(input, proxy.clone()) {
        drop(view);
        return Outcome::error(Failure::LoginFailed);
    }
    let deadline = Instant::now() + Duration::from_secs(request.timeout_seconds);
    let interval = Duration::from_millis(500);
    let mut next_probe = Instant::now();
    let mut pending: Option<Probe> = None;
    let mut music_page = false;
    let mut next_id = 0;
    let mut outcome = None;
    #[cfg(feature = "fixture")]
    let mut complete_fixture = false;
    event_loop.run_return(|event, _, flow| {
        let now = Instant::now();
        if now >= deadline {
            outcome = Some(Outcome::error(Failure::LoginTimeout));
            *flow = ControlFlow::Exit;
            return;
        }
        match event {
            Event::UserEvent(Signal::Cancel)
            | Event::WindowEvent {
                event: WindowEvent::CloseRequested,
                ..
            } => {
                outcome = Some(Outcome::cancelled());
            }
            Event::UserEvent(Signal::InputFailed) => {
                outcome = Some(Outcome::error(Failure::LoginFailed))
            }
            #[cfg(feature = "fixture")]
            Event::UserEvent(Signal::FixtureComplete) => {
                complete_fixture = true;
                next_probe = now;
            }
            Event::UserEvent(Signal::Focus) => {
                window.set_visible(true);
                window.set_minimized(false);
                window.set_focus();
                let _ = view.focus();
            }
            Event::UserEvent(Signal::Navigating) => {
                pending = None;
                music_page = false;
            }
            Event::UserEvent(Signal::Loaded { music }) => {
                // Invalidate callbacks from a page replaced by navigation.
                pending = None;
                music_page = music;
                next_probe = now;
            }
            Event::UserEvent(Signal::Metadata { id, raw })
                if pending.as_ref().is_some_and(|p| p.id == id) =>
            {
                let probe = pending.take().unwrap();
                if let Some(metadata) = Metadata::parse(&raw).filter(|_| {
                    music_page
                        && !loading.load(Ordering::SeqCst)
                        && probe.generation == generation.load(Ordering::SeqCst)
                }) {
                    match probe.stage {
                        Stage::Before => {
                            let now_seconds = SystemTime::now()
                                .duration_since(UNIX_EPOCH)
                                .unwrap_or_default()
                                .as_secs() as i64;
                            match view
                                .cookies()
                                .map_err(|_| ())
                                .and_then(|c| session::select(c, now_seconds))
                            {
                                Ok(Some(cookies)) => {
                                    next_id += 1;
                                    let id = next_id;
                                    pending = Some(Probe {
                                        id,
                                        generation: probe.generation,
                                        started: now,
                                        stage: Stage::After { metadata, cookies },
                                    });
                                    if !query(&view, &proxy, id) {
                                        outcome = Some(Outcome::error(Failure::LoginFailed));
                                    }
                                }
                                Ok(None) => {} // Keep waiting for an actual signed-in session.
                                Err(_) => outcome = Some(Outcome::error(Failure::LoginFailed)),
                            }
                        }
                        Stage::After {
                            metadata: before,
                            cookies,
                        } => {
                            if metadata == before {
                                outcome = Some(Outcome::session(Session {
                                    cookies,
                                    account_index: metadata.account_index,
                                    on_behalf_of_user: metadata.on_behalf_of_user,
                                }));
                            }
                        }
                    }
                }
                next_probe = now + interval;
            }
            _ => {}
        }
        if outcome.is_some() {
            *flow = ControlFlow::Exit;
            return;
        }
        if pending
            .as_ref()
            .is_some_and(|p| now.duration_since(p.started) >= Duration::from_secs(5))
        {
            pending = None;
        }
        if now >= next_probe {
            next_probe = now + interval;
            #[cfg(feature = "fixture")]
            if complete_fixture && music_page {
                if view
                    .evaluate_script("window.fixtureConfirmed=true")
                    .is_err()
                {
                    outcome = Some(Outcome::error(Failure::LoginFailed));
                    *flow = ControlFlow::Exit;
                    return;
                }
            }
            if pending.is_none() && music_page && !loading.load(Ordering::SeqCst) {
                next_id += 1;
                pending = Some(Probe {
                    id: next_id,
                    generation: generation.load(Ordering::SeqCst),
                    started: now,
                    stage: Stage::Before,
                });
                if !query(&view, &proxy, next_id) {
                    outcome = Some(Outcome::error(Failure::LoginFailed));
                    *flow = ControlFlow::Exit;
                    return;
                }
            }
        }
        *flow = ControlFlow::WaitUntil(next_probe.min(deadline));
    });
    // Only the caller emits the outcome, after this WebView and window are gone.
    drop(pending);
    drop(view);
    drop(window);
    #[cfg(windows)]
    {
        drop(context);
        if !profile.cleanup() {
            return Outcome::error(Failure::LoginFailed);
        }
    }
    outcome.unwrap_or_else(Outcome::cancelled)
}

#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn windows_private_mode_requires_a_known_supported_runtime() {
        assert!(!supported_windows_version("100.0.9999.99"));
        assert!(!supported_windows_version("101.0.1210.38"));
        assert!(supported_windows_version("101.0.1210.39"));
        assert!(supported_windows_version("140.0.0.0"));
        assert!(!supported_windows_version("unknown"));
        assert!(!supported_windows_version("140.0.0.0 beta"));
    }

    #[test]
    fn linux_cookie_enumeration_requires_a_known_supported_runtime() {
        assert!(!supported_linux_version("2.40.7"));
        assert!(supported_linux_version("2.42.0"));
        assert!(supported_linux_version("2.50.1"));
        assert!(!supported_linux_version("unknown"));
    }

    #[test]
    fn account_metadata_is_only_requested_from_the_music_https_origin() {
        assert!(music_url("https://music.youtube.com/"));
        assert!(music_url("https://music.youtube.com:443/browse"));
        for address in [
            "http://music.youtube.com/",
            "https://music.youtube.com.example.org/",
            "https://youtube.com/",
            "https://music.youtube.com:444/",
            "https://user@music.youtube.com/",
            "about:blank",
        ] {
            assert!(!music_url(address));
        }
    }
}

use crate::{OUTPUT_LIMIT, PROOF, Request, read_request, write_reply};
use serde_json::{Value, json};
use std::io;
use std::sync::{
    Arc,
    atomic::{AtomicBool, Ordering},
};
use std::time::{Duration, Instant};
use tao::event::Event;
use tao::event_loop::{ControlFlow, EventLoopBuilder};
use tao::platform::run_return::EventLoopExtRunReturn;
use tao::window::WindowBuilder;
use wry::{PageLoadEvent, WebViewBuilder};

enum Signal {
    Request(Request),
    Ready,
    Reply(String),
    End,
}

pub(super) fn run() -> Result<(), ()> {
    let mut events = EventLoopBuilder::<Signal>::with_user_event().build();
    #[cfg(target_os = "macos")]
    {
        use tao::platform::macos::{ActivationPolicy, EventLoopExtMacOS};
        events.set_activation_policy(ActivationPolicy::Accessory);
    }
    let window = WindowBuilder::new()
        .with_title("ytmgo script host")
        .with_visible(false)
        .build(&events)
        .map_err(|_| ())?;
    #[cfg(windows)]
    let mut profile = crate::windows_profile::Profile::new().map_err(|_| ())?;
    #[cfg(windows)]
    let mut context = wry::WebContext::new(Some(profile.path()));
    #[cfg(windows)]
    let builder = WebViewBuilder::new_with_web_context(&mut context);
    #[cfg(not(windows))]
    let builder = WebViewBuilder::new();
    #[cfg(target_os = "macos")]
    let builder = {
        use objc2::MainThreadMarker;
        use objc2_web_kit::{WKWebViewConfiguration, WKWebsiteDataStore};
        use wry::WebViewBuilderExtMacos;
        let mtm = MainThreadMarker::new().ok_or(())?;
        let config = unsafe { WKWebViewConfiguration::new(mtm) };
        let store = unsafe { WKWebsiteDataStore::nonPersistentDataStore(mtm) };
        unsafe {
            config.setWebsiteDataStore(&store);
        }
        builder.with_webview_configuration(config)
    };
    let ready = events.create_proxy();
    let reply = events.create_proxy();
    let initial = Arc::new(AtomicBool::new(true));
    let navigation = initial.clone();
    let builder = builder
        .with_incognito(true)
        .with_visible(false)
        .with_autoplay(false)
        .with_user_agent(include_str!("../assets/user-agent.txt").trim())
        .with_devtools(false)
        .with_navigation_handler(move |url| {
            navigation.load(Ordering::SeqCst)
                && matches!(
                    url.as_str(),
                    "about:blank"
                        | "https://www.youtube.com/"
                        | "https://www.youtube.com/index.html"
                )
        })
        .with_new_window_req_handler(|_, _| wry::NewWindowResponse::Deny)
        .with_download_started_handler(|_, _| false)
        .with_ipc_handler(move |request| {
            if request.body().len() <= OUTPUT_LIMIT {
                let _ = reply.send_event(Signal::Reply(request.into_body()));
            }
        })
        .with_on_page_load_handler(move |event, url| {
            if matches!(event, PageLoadEvent::Finished)
                && url.starts_with("https://www.youtube.com/")
            {
                initial.store(false, Ordering::SeqCst);
                let _ = ready.send_event(Signal::Ready);
            }
        })
        .with_initialization_script(PROOF);
    let html = "<!doctype html><html><head><meta http-equiv=\"Content-Security-Policy\" content=\"default-src 'none'; script-src 'unsafe-inline' 'unsafe-eval'; connect-src 'none'; media-src 'none'; object-src 'none'; frame-src 'none'; base-uri 'none'; form-action 'none'\"></head><body></body></html>";
    #[cfg(target_os = "linux")]
    let view = {
        use tao::platform::unix::WindowExtUnix;
        use wry::WebViewBuilderExtUnix;
        builder
            .build_gtk(window.default_vbox().ok_or(())?)
            .map_err(|_| ())?
    };
    #[cfg(not(target_os = "linux"))]
    let view = builder.build(&window).map_err(|_| ())?;
    #[cfg(target_os = "macos")]
    {
        use objc2_foundation::{NSString, NSURL};
        use wry::WebViewExtMacOS;
        let base =
            NSURL::URLWithString(&NSString::from_str("https://www.youtube.com/")).ok_or(())?;
        unsafe {
            view.webview()
                .loadHTMLString_baseURL(&NSString::from_str(html), Some(&base));
        }
    }
    #[cfg(target_os = "linux")]
    {
        use webkit2gtk::WebViewExt;
        use wry::WebViewExtUnix;
        view.webview()
            .load_html(html, Some("https://www.youtube.com/"));
    }
    #[cfg(windows)]
    {
        use webview2_com::Microsoft::Web::WebView2::Win32::{
            COREWEBVIEW2_HOST_RESOURCE_ACCESS_KIND_DENY, ICoreWebView2_3,
        };
        use windows_core::{HSTRING, Interface};
        use wry::WebViewExtWindows;
        // Map only our fixed empty page, never the WebView profile. The document
        // gets a stable HTTPS origin without fetching YouTube or a subresource.
        let folder = profile.path().join("proof-page");
        std::fs::create_dir(&folder).map_err(|_| ())?;
        std::fs::write(folder.join("index.html"), html).map_err(|_| ())?;
        let native = view.webview();
        let mapping: ICoreWebView2_3 = native.cast().map_err(|_| ())?;
        unsafe {
            mapping
                .SetVirtualHostNameToFolderMapping(
                    &HSTRING::from("www.youtube.com"),
                    &HSTRING::from(folder.as_os_str()),
                    COREWEBVIEW2_HOST_RESOURCE_ACCESS_KIND_DENY,
                )
                .map_err(|_| ())?;
            native
                .Navigate(&HSTRING::from("https://www.youtube.com/index.html"))
                .map_err(|_| ())?;
        }
    }
    let input = events.create_proxy();
    std::thread::spawn(move || {
        let mut stream = io::BufReader::new(io::stdin());
        while let Ok(Some(request)) = read_request(&mut stream) {
            if input.send_event(Signal::Request(request)).is_err() {
                return;
            }
        }
        let _ = input.send_event(Signal::End);
    });
    let mut loaded = false;
    let mut pending: Option<Request> = None;
    let mut active = 0u64;
    let mut last = 0u64;
    let mut began = Instant::now();
    let mut failed = false;
    events.run_return(|event,_,flow| {
        *flow=ControlFlow::WaitUntil(Instant::now()+Duration::from_millis(250));
        if (!loaded || active>0) && began.elapsed()>Duration::from_secs(30) { failed=true; *flow=ControlFlow::Exit; return }
        match event {
            Event::UserEvent(Signal::End) => { *flow=ControlFlow::Exit; return },
            Event::UserEvent(Signal::Ready) => { loaded=true; },
            Event::UserEvent(Signal::Request(request)) => {
                if pending.is_some() || active!=0 || request.id<=last { failed=true; *flow=ControlFlow::Exit; return }
                last=request.id; pending=Some(request);
            },
            Event::UserEvent(Signal::Reply(raw)) => {
                let value=serde_json::from_str::<Value>(&raw);
                if let Ok(value)=value {
                    if active>0 && value.get("id").and_then(Value::as_u64)==Some(active) && value.get("ok").and_then(Value::as_bool).is_some() {
                        if write_reply(&value).is_err() { failed=true; *flow=ControlFlow::Exit; return }
                        active=0;
                    } else { failed=true; *flow=ControlFlow::Exit; return }
                } else { failed=true; *flow=ControlFlow::Exit; return }
            },
            _=>{}
        }
        if loaded && active==0 {
            if let Some(request)=pending.take() {
                active=request.id; began=Instant::now();
                let code=json!(request.code).to_string();
                let script=format!("(async()=>{{try{{const v=await (0,eval)({code});window.ipc.postMessage(JSON.stringify({{id:{active},ok:true,value:v}}));}}catch(e){{const c=['proof_global_conflict','proof_interpreter_failed','proof_interpreter_missing','proof_program_failed','proof_snapshot_failed','proof_snapshot_invalid','proof_signals_missing'].includes(e?.message)?e.message:'script_failed';window.ipc.postMessage(JSON.stringify({{id:{active},ok:false,category:c}}));}}}})()");
                if view.evaluate_script(&script).is_err() { failed=true; *flow=ControlFlow::Exit; }
            }
        }
    });
    drop(view);
    drop(window);
    #[cfg(windows)]
    {
        drop(context);
        if !profile.cleanup() {
            failed = true;
        }
    }
    if failed { Err(()) } else { Ok(()) }
}

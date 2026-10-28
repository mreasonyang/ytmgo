#![cfg_attr(windows, windows_subsystem = "windows")]

mod app;
#[cfg(feature = "fixture")]
mod fixture_diagnostics;
#[cfg(target_os = "macos")]
mod macos_menu;
#[cfg(target_os = "macos")]
mod macos_webview;
mod protocol;
mod session;
#[cfg(windows)]
mod windows_profile;

use protocol::{Failure, Outcome, Request};
use std::io::{self, BufReader};

fn execute() -> Outcome {
    let mut input = BufReader::new(io::stdin());
    let line = match protocol::read_line(&mut input) {
        Ok(Some(line)) => line,
        Ok(None) => return Outcome::cancelled(),
        Err(_) => return Outcome::error(Failure::LoginFailed),
    };
    let request = match serde_json::from_slice::<Request>(&line) {
        Ok(request) if request.valid() => request,
        _ => return Outcome::error(Failure::LoginFailed),
    };
    app::run(request, input)
}

fn main() {
    // Never print panic payloads, page data or cookie values to diagnostics.
    std::panic::set_hook(Box::new(|_| {}));
    let outcome =
        std::panic::catch_unwind(execute).unwrap_or_else(|_| Outcome::error(Failure::LoginFailed));
    let _ = protocol::write_outcome(&outcome, &mut io::stdout().lock());
}

//! Fixed lifecycle markers for the offline fixture, never compiled in production.
use std::sync::atomic::{AtomicU32, Ordering};

#[derive(Clone, Copy)]
pub enum Stage {
    ViewCreated,
    StoreEmpty,
    CookieSet,
    LoadStarted,
    LoadExpected,
    LoadOther,
    CompleteReceived,
    MetadataRequested,
    MetadataReceived,
    MetadataValid,
    CookiesReadStarted,
    CookiesSelected,
    CookiesMissing,
    MetadataAfterRequested,
    MetadataAfterValid,
    SessionReady,
    CleanupStarted,
    CleanupFinished,
}

pub fn record(stage: Stage) {
    static SEEN: AtomicU32 = AtomicU32::new(0);
    let bit = 1 << stage as u32;
    if SEEN.fetch_or(bit, Ordering::Relaxed) & bit != 0 {
        return;
    }
    let name = match stage {
        Stage::ViewCreated => "view_created",
        Stage::StoreEmpty => "store_empty",
        Stage::CookieSet => "cookie_set",
        Stage::LoadStarted => "load_started",
        Stage::LoadExpected => "load_expected",
        Stage::LoadOther => "load_other",
        Stage::CompleteReceived => "complete_received",
        Stage::MetadataRequested => "metadata_requested",
        Stage::MetadataReceived => "metadata_received",
        Stage::MetadataValid => "metadata_valid",
        Stage::CookiesReadStarted => "cookies_read_started",
        Stage::CookiesSelected => "cookies_selected",
        Stage::CookiesMissing => "cookies_missing",
        Stage::MetadataAfterRequested => "metadata_after_requested",
        Stage::MetadataAfterValid => "metadata_after_valid",
        Stage::SessionReady => "session_ready",
        Stage::CleanupStarted => "cleanup_started",
        Stage::CleanupFinished => "cleanup_finished",
    };
    // All bytes are fixed literals, no page, account, cookie or diagnostic values.
    eprintln!("ytmgo-fixture-stage:{name}");
}

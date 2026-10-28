//! Preserve WebKit's system user agent and add its Safari compatibility token.
use objc2::{MainThreadMarker, rc::Retained};
use objc2_foundation::NSString;
use objc2_web_kit::{WKWebViewConfiguration, WKWebsiteDataStore};

pub fn configuration() -> Option<Retained<WKWebViewConfiguration>> {
    let mtm = MainThreadMarker::new()?;
    // WKWebView on macOS omits the Safari product token. YouTube Music serves
    // an unsupported-browser page for that bare UA, without account metadata.
    // Append the frozen WebKit/Safari compatibility token and our own identity;
    // do not replace the system UA or claim a Chrome or Safari browser version.
    // https://bugs.webkit.org/show_bug.cgi?id=278284
    let suffix = NSString::from_str(concat!("Safari/605.1.15 ytmgo/", env!("CARGO_PKG_VERSION")));
    // SAFETY: WK configuration and store APIs are used only on the main thread.
    unsafe {
        let configuration = WKWebViewConfiguration::new(mtm);
        // Wry uses a supplied configuration's store instead of its incognito
        // attribute. Set the private store explicitly before constructing it.
        let store = WKWebsiteDataStore::nonPersistentDataStore(mtm);
        if store.isPersistent() {
            return None;
        }
        configuration.setWebsiteDataStore(&store);
        configuration.setApplicationNameForUserAgent(Some(&suffix));
        Some(configuration)
    }
}

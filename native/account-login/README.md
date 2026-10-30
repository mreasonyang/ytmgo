# Temporary account window

The MyGo application starts this helper only when the user chooses Sign in.
Wry uses WKWebView on macOS, WebView2 on Windows, and WebKitGTK on Linux. The
window uses a private WebView context. It does not launch a media engine or
access the application's saved account.

Google's official sign-in page handles account selection and verification.
Once the top-level Music page supplies an explicit account index, the helper
reads the native cookie store, filters the YouTube session, and checks the
page's account metadata again. Cookies are never read through `document.cookie`.
The parent validates the returned session with YouTube before saving it in the
existing encrypted account store.

Communication uses inherited stdin/stdout pipes, with a single bounded result.
No login HTTP listener, browser extension, OAuth registration, cookie file, or
system-browser import is involved. Interactive login sends `timeoutSeconds: 0`
and imposes no application-defined expiration. Closing the window, cancellation,
or closing the parent's pipe ends the helper; tests and bounded callers may
request an explicit timeout of 1 to 1800 seconds. The parent waits for helper exit
before accepting the result. Raw process output must never be logged.

## Platform requirements

- macOS: system WKWebView. The application package determines the minimum OS.
  A nested `ytmgo Account Login.app` supplies the helper's own application
  identity and accessibility target; it still runs only for the login lifetime.
  It uses `LSUIElement` and Tao's accessory activation policy so the login
  window can receive focus without adding a Dock or Command-Tab entry.
- Windows: Microsoft Edge WebView2 Runtime 101.0.1210.39 or newer. An older
  runtime can ignore private mode, so the helper rejects it before opening a
  window. A separate temporary user-data directory supports installation in
  read-only Program Files; it is removed after the private WebView closes.
- Linux: GTK 3 and WebKitGTK 4.1 >=2.50.2, including development packages when compiling.
  The GTK container integration supports both X11 and Wayland.

Passkey support is a separate platform capability. macOS WKWebView requires a
relationship with the relying party through Associated Domains; this helper
cannot authenticate Google passkeys. Users need another method offered by the
Google page. Apple's browser-only entitlement is not applicable to this music
client. Windows WebView2 supports FIDO2, but this app's real Google passkeys have
not been tested. WebKitGTK 2.50.2 fixes Google's alternative-method flow when
WebAuthn is unavailable; it does not add passkey support.

Sources: [Apple passkeys](https://developer.apple.com/documentation/authenticationservices/supporting-passkeys),
[browser entitlement](https://developer.apple.com/documentation/bundleresources/entitlements/com.apple.developer.web-browser.public-key-credential),
[Microsoft FIDO2](https://learn.microsoft.com/en-us/entra/identity-platform/support-fido2-authentication),
[WebKitGTK release](https://webkitgtk.org/2025/11/19/webkitgtk2.50.2-released.html).

The main application keeps the encryption key in its platform credential store:
Keychain, Windows Credential Manager, or Linux Secret Service. A missing
credential service is an error; there is no plaintext storage alternative.

Build on each destination OS with `python3 scripts/login/prepare.py` from the
repository root. The staging manifest checks the target platform and CPU
architecture. The media-runtime packager is separate and currently macOS-only;
successful helper builds do not establish complete Windows/Linux app delivery.

## Verification

`cargo test --locked` checks the protocol, cookie filtering and account metadata.
The `fixture` feature builds a fixed offline page and an HttpOnly synthetic
cookie in the real WebView store. It adds a fixed `fixture-complete` command for
`scripts/login/test_lifecycle.py`; production builds reject that command and
contain no fixture page. The fixture checks handoff, cancellation, parent-pipe
closure and timeout. It never uses a Google account.

Native helper tests, real Google sign-in, complete browser-process cleanup,
and full application playback are separate acceptance checks. Results belong
in the repository's VERIFICATION.md.

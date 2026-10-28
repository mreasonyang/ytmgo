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
system-browser import is involved. Closing the window, cancellation, timeout,
or closing the parent's pipe ends the helper. The parent waits for helper exit
before accepting the result. Raw process output must never be logged.

## Platform requirements

- macOS: system WKWebView. The application package determines the minimum OS.
- Windows: Microsoft Edge WebView2 Runtime 101.0.1210.39 or newer. An older
  runtime can ignore private mode, so the helper rejects it before opening a
  window. A separate temporary user-data directory supports installation in
  read-only Program Files; it is removed after the private WebView closes.
- Linux: GTK 3 and WebKitGTK 4.1 >=2.42, including development packages when compiling.
  The GTK container integration supports both X11 and Wayland.

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

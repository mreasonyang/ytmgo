System playlist covers now display their official artwork, including Liked Music and generated mixes such as Discover Mix. Saved libraries load automatically after account restoration, and native account controls show the signed-in avatar.

Other changes since 0.5.1:

- Replace the Node playback runtime with Go and a native script host.
- Use configured proxies consistently for catalogue, account, artwork and playback requests; retry temporary artwork failures.
- Reopen playback streams after system sleep.
- Group settings into cards and support a configurable global playback shortcut.
- Load the next page on scroll and recover an existing account-login window when reopening the app.

macOS 27 or newer, Apple Silicon (arm64). The App and DMG are Developer ID signed, notarized and stapled. The automatic update package has a separate Ed25519 signature. Existing installations can check for updates from the ytmgo menu; installations on 0.4.0 require a manual DMG update.

Source: [c15f02a](https://github.com/mreasonyang/ytmgo/commit/c15f02a005f25f97d3dd09e355119dad2a3c58cc).

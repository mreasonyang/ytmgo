# ytmgo 0.6.4

macOS now targets **macOS 13 Ventura or newer**, on **Apple Silicon and Intel**.

Download the [Apple Silicon DMG](https://github.com/mreasonyang/ytmgo/releases/download/v0.6.4/ytmgo-0.6.4-arm64-notarized.dmg) or [Intel DMG](https://github.com/mreasonyang/ytmgo/releases/download/v0.6.4/ytmgo-0.6.4-amd64-notarized.dmg).

- Separate native arm64 and amd64 DMG/ZIP downloads; both include the native login helper, playback host and FFmpeg.
- Every embedded Mach-O deployment target and architecture is checked before signing. FFmpeg now uses an explicit macOS 13 target instead of inheriting the build machine's OS version.
- Both apps and DMGs are Developer ID signed, notarized and stapled. Each architecture has its own Ed25519-signed update archive and feed.
- Publication requires both signed apps to pass native installation/startup checks on an Apple Silicon macOS 27 runner and an Intel macOS 15 runner. These checks cover the app, decoder, script host, login-helper protocol and Gatekeeper acceptance in a fresh profile.

The public player transform is now cached across restarts and account changes, reducing repeated downloads and extraction during playback preparation.

The macOS 13 minimum follows the current Go toolchain requirement. Deployment-target validation is separate from runtime acceptance: macOS 13/14 and a signed-in account on Intel have not been exercised on physical machines in this release pipeline.

Homebrew installation:

```sh
brew tap mreasonyang/taps
brew install --cask ytmgo
```

Homebrew selects the native architecture. Existing installations can use `brew upgrade --cask --greedy ytmgo` or the app's signed updater. Both public Homebrew packages and the public 0.6.3 → 0.6.4 ARM upgrade have passed native installation/startup checks.

## Windows and Linux downloads

Windows x64: use the setup EXE or portable ZIP. WebView2 Runtime is required for sign-in and non-Premium playback. The Windows installer is not Authenticode signed.

Linux x64: use the DEB on Ubuntu 24.04 or compatible systems, or extract the Linux tar.gz. GTK 3, WebKitGTK 4.1 (2.50.2+), Secret Service and a desktop display/audio session are required.

Both platforms include native playback/login helpers, licenses and corresponding sources. Their update archives carry Ed25519 signatures; installer/startup checks passed on the corresponding OS. Real-account UI/audio E2E and long-running acceptance remain unverified.

Source: 23b4118cd459de7f12ebcee734cbd99e704f6457

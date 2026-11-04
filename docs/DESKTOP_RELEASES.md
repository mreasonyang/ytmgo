# Windows and Linux releases

Windows and Linux packages supplement the existing macOS release using the exact public tag's application code. Their platform-specific checksums and build metadata are separate from the Mac files. Supplemental third-party license notices are pinned to upstream revisions and SHA-256 values; the packaging-tools revision and legal-data additions are listed separately in each build receipt.

## Windows x64

Download `ytmgo-setup-<version>-amd64.exe` from [Releases](https://github.com/mreasonyang/ytmgo/releases/latest), or extract the complete `ytmgo-<version>-windows-amd64.zip` and run `ytmGO.exe`. Keep `runtime`, `login`, `legal` and the other packaged resources beside the executable.

The installer runs per user. A current Microsoft WebView2 Runtime is required for Google sign-in and ordinary/anonymous playback. The installer is **not Authenticode signed**. The app's in-place update archives have a separate Ed25519 signature, checked with the existing embedded public key before installation.

## Linux x64

The DEB is built and smoke-tested on Ubuntu 24.04 x64:

```sh
sudo apt install ./ytmgo_<version>_amd64.deb
```

Launch ytmGO from the applications menu, or run `/opt/ytmgo/ytmgo`. For the tar archive, extract its complete contents to a writable directory and run `./ytmgo` there. Keep all packaged resource directories together.

Linux needs GTK 3, WebKitGTK 4.1 **2.50.2+**, libsecret/Secret Service and a working desktop display/audio session. The DEB declares its GTK/WebKitGTK/libsecret dependencies. Other distributions must supply compatible system libraries; this Ubuntu build is not a statically linked universal Linux application. Native startup in a fresh profile and live signed-in account/audio E2E are separate acceptance levels.

Platform checksums are `SHA256SUMS-windows-amd64` and `SHA256SUMS-linux-amd64`. Build receipts are `build-windows-amd64.json` and `build-linux-amd64.json`. These receipts bind the application source revision, native build, actual installer/startup smoke and signing state to the release.

## Publishing

The **Windows and Linux packages** workflow runs automatically after a successful **macOS package** workflow when its source revision matches the latest public stable release. An unsigned Mac build or a release candidate cannot cause the workflow to publish packages for a different source revision. Completed platform releases are detected and skipped.

For an existing release, dispatch it explicitly:

```sh
gh workflow run desktop-package.yml --repo mreasonyang/ytmgo --ref main \
  -f release_tag=v0.6.4 -f publish_release=true
```

The workflow resolves the public release's `build.json`, checks its GitHub digest and immutable source revision, then checks out that source separately from the current packaging tools. Native Windows/Linux runners build their own FFmpeg, script host, login helper and legal resources. They receive no signing credentials or real accounts. The workflow installs the NSIS/DEB package, verifies installed resources and version, runs the packaged helpers, and checks application startup in an empty account profile.

Accepted archives return to the existing `macos-release` environment on the release Mac for update signing. The key goes to the signing helper on stdin. Publication checks signatures, source identity, smoke receipts and checksums; existing assets are never overwritten. Additional platform files are uploaded without changing the Mac files, checksums or Git tag.

The first Windows/Linux release has no previous native package baseline for a real version-to-version upgrade. Signature/feed verification and installer startup are recorded separately from future native upgrade, signed-in UI, hardware audio, keyboard, sleep/wake and long-run acceptance.

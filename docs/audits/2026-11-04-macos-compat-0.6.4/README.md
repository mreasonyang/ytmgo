# macOS deployment target and Intel release — 0.6.4

Release source: `23b4118cd459de7f12ebcee734cbd99e704f6457`.

PASS: signed/notarized dual-architecture release, native startup, public updater
verification, actual Homebrew installation on both CPUs, and public ARM upgrade.

## Cause and correction

The 0.6.3 app plist required macOS 27. The main Go executable actually targeted
13 and the Rust helpers targeted 11, but FFmpeg inherited macOS 27 from its
build host. Lowering only the plist would therefore leave a broken compatibility
claim. [Installed 0.6.3 component evidence](before.json).

The app and login plist now declare 13.0. Rust receives
`MACOSX_DEPLOYMENT_TARGET=13.0`; FFmpeg receives explicit compiler and linker
`-mmacosx-version-min=13.0` flags and uses a separate cache directory. The
[Go 1.27 platform requirement](https://go.dev/doc/go1.27) sets the current lower
bound at macOS 13 Ventura.

Packaging paths, DMG/ZIP names, build metadata, update archives and feeds now
cover both `arm64` and `amd64`. Each unsigned bundle is inspected before
signing; every shipped Mach-O must have the expected native CPU and a deployment
target no newer than 13.0. Nested plist minima and non-system native library
references are also checked.

Native builders have no signing credentials. Both architectures are signed and
notarized on the existing protected `macos-release` ARM job. A subsequent pair
of native runners installs the signed app, checks Gatekeeper and stapled tickets,
runs the app/version command, script host, FFmpeg and login-helper EOF protocol,
and requires twelve seconds of startup plus initialized audio diagnostics in a
fresh profile. Publication additionally binds both smoke receipts to the exact
signed ZIP SHA-256 and verifies both Ed25519 updater signatures.

The final source also includes the separately committed
[public player transform cache repair](../2026-11-04-player-cache/README.md).

## Validation

- Local: 36 release script tests, 10 login staging tests, 7 licensing tests and
  Go update-package signing/roundtrip/tamper/architecture tests passed.
  [Receipt](local-tests.json).
- [First run](https://github.com/mreasonyang/ytmgo/actions/runs/41842662566):
  Intel build, Go/vet and offline media-runtime tests passed. The smoke launcher
  then failed because it changed cwd while retaining a relative executable path.
  The release remained unpublished. Absolute-path resolution and a regression
  test fix this; the unavailable macOS 15 ARM queue was replaced with `xcode-27`.
  [Failure record](first-attempt.json).
- [Release run](https://github.com/mreasonyang/ytmgo/actions/runs/41845608227):
  both builds, runtime tests, deployment inspections, notarization and signed
  native startup passed. [ARM receipt](signed-smoke-arm64.json) ·
  [Intel receipt](signed-smoke-amd64.json).
- The first publication attempt failed at draft creation after main advanced.
  Pinning the tested SHA to `release/0.6.4-source` allowed the same publish job
  to complete without rebuilding or changing any payload. GitHub documents
  [workflow scope requirements for unreferenced workflow-changing commits](https://github.blog/changelog/2023-11-02-github-actions-enforcing-workflow-scope-when-creating-a-release/).
  Future formal release runs pin their source before building; 38 release-script
  regressions passed, including refusing to move an existing source pin.
  [Record](source-pin-regressions.json).
- Independent anonymous downloads: both updater TARs, both feeds, both build
  receipts and checksums passed GitHub asset-digest comparison. Both Ed25519
  signatures, app signatures, tickets, Gatekeeper assessments and every embedded
  architecture/deployment target passed. [Public verification](public-verification.json).
- [Intel WebView lifecycle](https://github.com/mreasonyang/ytmgo/actions/runs/41848250319)
  passed synthetic sign-in, cancellation, timeout and cleanup. Its login-helper
  inputs are [identical to the release](intel-login-source.json).
- [Actual Homebrew installs](https://github.com/mreasonyang/homebrew-taps/actions/runs/41850617166)
  passed on separate ARM and Intel machines using the public ZIPs:
  [ARM](homebrew-arm64.json) · [Intel](homebrew-amd64.json).
  The Homebrew receipt's `commit` field identifies the tap workflow revision
  `aa1508c`; its checked-out app tag is v0.6.4 at the release SHA above.
- [Public 0.6.3 → 0.6.4 ARM upgrade](https://github.com/mreasonyang/ytmgo/actions/runs/41850200341)
  passed; the installed result matched the independently downloaded public ZIP,
  with valid signatures, ticket and Gatekeeper acceptance.
  [Receipt](public-upgrade-receipt.json).
- [Windows/Linux native packages](https://github.com/mreasonyang/ytmgo/actions/runs/41849890402)
  passed native builds, installers, fresh-profile startup and update signing.
  The final release has 22 assets; all 11 Mac asset digests remain unchanged
  after the additional desktop files were published. [Final asset check](final-asset-check.json).
  Windows/Linux artifacts were verified by their release workflow; this audit's
  independent updater downloads/signature checks cover the two Mac architectures.

## Acceptance boundary

Native startup environments are macOS 27.0.1 Apple Silicon and macOS 15.7.9 Intel.
A macOS 13 target is verified for every shipped executable, but macOS 13/14
runtime acceptance has not been performed. Intel real-account UI/playback,
hardware audio, shortcuts and sleep/wake are outside the fresh-profile startup
checks. Rosetta is not used as native Intel acceptance. This task does not
replace the user's currently playing `/Applications` installation.

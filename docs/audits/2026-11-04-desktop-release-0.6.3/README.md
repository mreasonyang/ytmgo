# Windows/Linux 0.6.3 supplemental release

**PASS for native Windows/Linux installer startup and public-package integrity.** Both platforms are now available in the same [v0.6.3 release](https://github.com/mreasonyang/ytmgo/releases/tag/v0.6.3). [Native package workflow](workflow.json) · [Independent public verification](public-verification.json)

Application code is bound to the existing public v0.6.3 revision, `a582d25bb3f5b793f4d3e57160db3e4c90f86099`. Packaging tools and supplemental legal-data inputs have their own provenance in each platform's `build-*.json`; the original Git tag is not moved.

| Platform | Files |
| --- | --- |
| Windows x64 | [Setup EXE](https://github.com/mreasonyang/ytmgo/releases/download/v0.6.3/ytmgo-setup-0.6.3-amd64.exe), [portable ZIP](https://github.com/mreasonyang/ytmgo/releases/download/v0.6.3/ytmgo-0.6.3-windows-amd64.zip), signed-update tar archive/feed, checksums and build receipt |
| Linux x64 | [DEB](https://github.com/mreasonyang/ytmgo/releases/download/v0.6.3/ytmgo_0.6.3_amd64.deb), [tar.gz](https://github.com/mreasonyang/ytmgo/releases/download/v0.6.3/ytmgo-0.6.3-linux-amd64.tar.gz), signed-update feed, checksums and build receipt |

The [final workflow](https://github.com/mreasonyang/ytmgo/actions/runs/41834832878) passed both native build/install/startup jobs and the Mac signing/publication job. Independent anonymous downloads verified all 11 new assets against GitHub's asset digests and the platform checksum lists. Both update archives passed Ed25519 verification, and both stable feeds matched their versioned manifests. All six original Mac asset digests remain unchanged. Windows archives contain 541 files and Linux archives 765; PE/ELF architecture checks passed and the Windows ZIP/TAR file inventories matched.

The workflow builds natively on Windows and Ubuntu 24.04 x64, stages the corresponding FFmpeg/script host/login helper, checks executable architectures, and verifies licenses/corresponding sources. Offline runtime protocol/lifecycle tests run on each target OS. It then installs the NSIS/DEB package, verifies the installed version and legal resources, runs the installed script-host/FFmpeg checks, and requires the real application process to remain alive for 12 seconds and initialize its fixed diagnostic file in an empty account profile.

Update archives return to the existing Mac release environment for Ed25519 signing and independent public-key verification. The Windows installer is **not Authenticode signed**. Linux is an Ubuntu 24.04 build requiring compatible GTK 3, WebKitGTK 4.1 2.50.2+, Secret Service and desktop/audio system libraries.

The [first attempt](first-attempt.json) built both playback/login helpers but stopped at missing original Rust dependency notices. Exact upstream revisions and SHA-256-pinned permission texts were added for dlopen2 and WebView2 crates; target-specific dependency/license selections then passed locally for both OSes. These license data additions are recorded separately from the unchanged application code.

The [second attempt](second-attempt.json) completed the Linux installer/startup checks. Windows stopped at pinned-notice staging. A local Windows-style checkout reproduced the checksum mismatch by converting LF to CRLF. The repository now preserves the original notice bytes and the Windows runner disables automatic newline conversion before checking out both source trees. The regression test preserves both originally-LF and originally-CRLF notice fixtures.

[Preflight checks](preflight.json) include 33 release-script tests, six licensing tests, synthetic update-signature roundtrips/tamper rejection and target-specific license metadata checks. The latter ran on Mac and are not native application execution evidence. Original v0.6.3 application source CI is recorded in the [Mac release audit](../2026-11-04-release-0.6.3/README.md).

Installed-process smoke is narrower than complete native UI/E2E. This run does not establish real Google account sign-in, signed-in/Premium playback, hardware audio, physical shortcuts, sleep/wake, long-run reliability or a prior-native-version Windows/Linux upgrade. No real account or signing credential is given to hosted Windows/Linux builders.

See [installation and maintenance](../../DESKTOP_RELEASES.md). Diagnostics retain fixed categories and allowlisted workflow/build metadata only; raw logs, process argv, environments and authentication stores are not published.

To reproduce downloads and verification, run [fetch_public.py](fetch_public.py), then [verify_public.py](verify_public.py). The fetcher uses official GitHub HTTP ranges in parallel and verifies complete-file SHA-256 values, addressing the observed slow single-connection transfer without changing system network settings.

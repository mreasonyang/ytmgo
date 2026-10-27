# Portable UI / UX / i18n audit evidence

This directory is committed to the repository so a fixing agent on another device can inspect the evidence without access to the auditor's Mac, local files, running applications, or loopback services.

Read the canonical [UIUX_AUDIT.md](../../../UIUX_AUDIT.md) for all nine findings, reproduction steps, acceptance criteria, and verification boundaries. See [VERIFICATION.md](../../../VERIFICATION.md) for the project verification record.

Audited product commit: `55880f11d28a833e1ed138c575ea79b750919531`, version 0.3.0, macOS 27.0.1 arm64. These are historical observations of that exact build, not proof about a later fix.

## Contents

- `screenshots/`: 15 original application-window screenshots selected from the local audit. English, Simplified Chinese, Traditional Chinese, and Japanese examples are included.
- `layout/`: six JSON snapshots from an isolated anonymous native WebKit diagnostic application loading the product shell. They contain layout rectangles, computed colors, visible control labels, display state, and media time/volume metadata.
- [native-observations.json](native-observations.json): explicit observations from the actual packaged application, including four language-triggered exits and the same-language preference-state failure.
- [manifest.json](manifest.json): asset paths, byte counts, and SHA-256 checksums.

The diagnostic application's custom native menu and English-only language payload are not evidence for the production application's native language handling. F01, F03, and F04 were checked in the actual packaged application. No credentials, account identity, authentication storage, process arguments, raw logs, or environments are included.

## Finding-to-evidence index

| Finding | Primary evidence |
| --- | --- |
| F01: language changes exit the application | `native-observations.json`; `02-native-zh-hans-dark.jpg` and `16-hant-shortcuts.jpg` show the persisted language after relaunch |
| F02: video height doubles and clips | `24-probe-wide-video-cropped-dark.jpg`, `25-probe-wide-immersive-video-cropped.jpg`; `wide-video-dark.json`, `wide-immersive-video-dark.json` |
| F03: saved language and selected option diverge | `28-native-same-locale-stale-choice.jpg`; `native-observations.json` |
| F04: Match System shows the manually chosen language | `15-ja-settings-system-hint.jpg`; system-English restoration in `native-observations.json` |
| F05: compact navigation remains covered by immersive mode | `13-ja-immersive-nav-no-visible-change.jpg`, `14-ja-after-immersive-nav.jpg` |
| F06: compact Queue/Lyrics entry points are missing | `22-probe-420-playing.jpg`, `12-ja-compact-immersive-minheight.jpg`, `23-probe-420-immersive.jpg`; `420-immersive-ad.json` |
| F07: light-theme text is unreadable | `26-probe-track-menu-light.jpg`, `27-probe-1100-overlay-light.jpg`; `wide-track-menu-light.json`, `1100-panel-light.json` |
| F08: Autoplay uses an inconsistent blue accent | `24-probe-wide-video-cropped-dark.jpg`, `26-probe-track-menu-light.jpg` |
| F09: compact idle layout reserves unused player space | `18-probe-420-home-dark.jpg`; `420-settings.json` |

Screenshot filenames above are inside `screenshots/`; layout JSON filenames are inside `layout/`.

## Important interpretation details

- F02 is a measured geometry mismatch: the docked frame was 328×184.5 CSS px while the internal video/container was 328×369; immersive mode showed the same 2× internal height. Distinguish this from normal letterboxing.
- For F07, the visible “More from YouTube” text was approximately 1.07:1 against its light background. The automated scanner also flagged some service-menu ancestors whose actual visible text was dark and readable. Those menu flags were excluded from the findings; inspect the screenshots before treating a contrast result as a defect.
- Original screenshots and observations should remain intact. Store verification of fixes separately and record the tested commit, platform, and application build.
- A remote agent without macOS can implement and run available checks, but should mark native macOS acceptance as pending. Browser or unit tests alone do not establish that the native language lifecycle and WebKit rendering issues are fixed.

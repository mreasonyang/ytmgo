# Settings and Keyboard shortcuts — UI/UX test and fixes

Date: 2026-10-30. Scope: the redesigned Settings sheet and the Keyboard
shortcuts sheet (`native_settings.go`, `global_shortcut.go`). Evidence is the
MyGo software renderer on Linux (`YTMGO_UI_SNAPSHOT_DIR`), so modifiers render
as Ctrl; macOS renders ⌃ ⌥ ⇧ ⌘. A native macOS window was not driven.

## Coverage

- Widths 380 (minimum), 420, 761, 900, 1100, 1280; heights 640 (minimum) to 900.
- English, Simplified Chinese, Traditional Chinese, Japanese; dark and light.
- Account: Premium, standard, signed out, restoring, slow restore, deferred.
- Global shortcut: default, off, unavailable, recording, recording with error.
- Keyboard: full Tab cycle, arrows, Escape in the sheet, the drop-down and
  the recorder; pointer: hover, backdrop click, clicking away while recording.
- Language drop-down in a short window; contrast of every text/fill pair.

## Findings and fixes

| ID | Severity | Finding | Fix |
| --- | --- | --- | --- |
| S01 | P2 | Recording did not end when focus left: after clicking another control the field kept saying "Press shortcut…" and swallowed keys. | The recorder takes focus once and recording is cancelled when it loses it. |
| S02 | P2 | At 380 DIPs the CJK appearance labels (跟随系统, システム, ダーク) wrapped inside their segments. | Narrow controls span the row; segment labels are single-line with tighter padding. |
| S03 | P2 | Appearance was three Tab stops of plain buttons. | `SegmentedBase`: one Tab stop, arrows/Home/End choose, announced as a radio group. |
| S04 | P2 | The sign-out row's focus ring was clipped by its card (only a bottom line showed). | The row is inset 4 DIPs with its own radius. |
| S05 | P2 | With the shortcut off the field just said "Off": no visible way to set one. | Off, the row offers "Record shortcut" by name in place of "Turn off". |
| S06 | P3 | Keyboard shortcuts showed ⌘/⇧ on Linux and Windows; modifiers kept stored order, not the platform's. | Chords are accelerators spelled by `shortcutDisplay`, which orders ⌃ ⌥ ⇧ ⌘ on macOS and Ctrl Alt Shift elsewhere. |
| S07 | P3 | "Premium" badge repeated "YouTube Music Premium", and white-on-accent 11 px text was 4.17:1 in light mode. | Badge removed; the status names the plan and the accent avatar marks it. |
| S08 | P3 | The global shortcut hint wrapped to three lines with an orphaned "off.". | Shorter hint ("Works even while you use other apps."); the recorder and errors explain the rest. |
| S09 | P3 | "Configure global shortcut" opened Settings at the top; in short or compact windows the shortcut was off-screen. | Settings opens with the Keyboard card scrolled into view. |
| S10 | P3 | No cue that the sheet scrolls; content cut flat at the bottom edge. | A hairline under the title once scrolled and a fade at the bottom while more remains; sheets reopen at the top. |
| S11 | P3 | The language drop-down reported its value as a code ("en") to assistive technology. | The drop-down's values are the names shown. |
| S12 | P3 | Compact title used 24 pt; DESIGN.md specifies 20 compact. | 20 pt at 760 and below. |
| S13 | P3 | Account actions squeezed slow/deferred restore text into a narrow column. | Actions go under the text whenever a restore notice shows. |

Checked and fine: Escape in the drop-down closes only the drop-down; Escape
while recording ends only recording; backdrop click closes the sheet; the
drop-down opens upward near the bottom of a short window; all remaining text
pairs are at least 4.5:1 (icons at least 3:1).

Not an app issue: Japanese kana render undersized in the Linux test renderer
(font fallback); the same happens outside Settings.

## Regression tests

`native_settings_test.go` covers S01–S06 and S09: appearance labels fit at
380 in all four languages, arrows choose appearance, the recorder ends on
blur and Escape keeps Settings open, the off state offers recording, the
sign-out row is inset, Configure reveals the shortcut clear of the fade, and
platform chord order. Each test was checked to fail with its fix reverted.

## Snapshots

[desktop dark](snapshots/desktop-dark-en.png) ·
[desktop light, 简体中文](snapshots/desktop-light-zh-Hans.png) ·
[380, 日本語](snapshots/compact-380-ja.png) ·
[shortcut off, light](snapshots/desktop-light-off.png) ·
[Keyboard shortcuts](snapshots/shortcuts-desktop-en.png)

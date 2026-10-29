# Validation before publishing the remaining local fixes

Date: 2026-10-29, Asia/Shanghai.

This delivery publishes the previously local playback, catalog, lyrics, and layout fixes together with their tests and the two earlier audit directories. These changes were already present during the comprehensive UI review. They do not constitute remediation of its six findings; those findings still require verification against the latest code.

The changes include page-bound visitor context for pagination, idempotent anonymous session clearing, localized metadata filtering, late audio seeking, bounded video startup and recovery, video-only retry, short desktop immersive layout improvements, and provider-timed lyrics with follow controls and plain-text fallback.

Fresh local validation passed:

- `make check`: Go tests, runtime and login checks, vet, and i18n validation.
- `go test -race -json ./...`: exit 0, no failing test events, 76.343 seconds. The 696 named pass events include parent tests and subtests.
- `make build`: exit 0, 24.019 seconds.
- `codesign --verify --deep --strict`: passed for the generated app.
- `hdiutil verify`: passed for the generated DMG.
- Formatting checks for all 29 changed/new Go files and `git diff --check`: passed.

The new local build is version 0.5.0 with an ad-hoc signature. Its app and DMG hashes, command results, and pre-commit source hashes are recorded in [delivery-validation.json](delivery-validation.json). It was not installed or publicly released.

This step did not repeat native account or playback acceptance on the newly built 0.5.0 app. Earlier native observations remain tied to the executable hashes in their original reports. The current comprehensive review is [here](../2026-10-29-full-ui-visual/README.md); its NO-GO verdict and evidence limits remain applicable to that audited candidate.

No credentials were read or copied. Publication review found no credential-pattern matches in the pending documentation. The media URL pattern in the new runtime test belongs to a synthetic `fixture.googlevideo.com` URL with an expiry parameter, not a captured playback URL.

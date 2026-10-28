# Service verification recovery — 2026-10-28

- `native-observations.json`: actual CUA interactions with the native AUDIT app and fixed state readbacks. Account flags and HTTP responses are synthetic.
- `diagnostic-build.json`: source hashes, diagnostic executable hash, fixture boundaries and build flags.
- `audit-entry.json`: exact independent diagnostic entry-point patch stored as a JSON string with its SHA256; it is not part of the production build.
- `artifact-recovery.json`: formal app/DMG integrity and content audit. The app was not launched; ad-hoc app signing and an unsigned DMG remain distribution limitations.

The browser action opened the fixed YouTube Music homepage in Brave. No real CAPTCHA, real account verification or successful transfer of browser challenge state is claimed. No credentials, upstream response bodies or private redirect URLs are included.

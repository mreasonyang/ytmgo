# ytmgo account extension

The native Sign In window opens the public unpacked extension folder and copies the public `brave://extensions/` address. Paste it into Brave's address bar, enable Developer mode, choose Load unpacked, and select that folder. Open YouTube Music in Brave, select the intended account, open **ytmgo Account**, confirm the selected account, and click **Connect account**. Keep the popup open until it confirms receipt; the app separately verifies sign-in and Music membership with YouTube.

This MV3 extension has no background worker, storage permission, or automatic content scripts. Explicit clicks read only `SESSION_INDEX` and `DELEGATED_SESSION_ID` from the selected active Music tab and query allowlisted cookies for that tab's cookie store. Only nonpartitioned parent-domain `.youtube.com` cookies with `/` scope are transferred. Missing required cookies fail closed.

The native app accepts the pinned extension origin on `127.0.0.1:52963` for five minutes. A random handshake token is fetched only in popup memory and sent in a request header; it is never in a URL or saved. One successful transfer closes the channel. The extension never persists credentials. The app saves verified sessions in its own system secret store, with no plaintext alternative. Signing out removes only this app's session and does not sign Brave out.

The manifest key is public and pins extension ID `llinflbckniejeiomanmglbdganiccfk`; it is not a secret. This source replaces the temporary diagnostic extension with the same ID. Synthetic tests: `node --test scripts/account-extension/session-policy.test.mjs`.

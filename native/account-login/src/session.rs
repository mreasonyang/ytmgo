use serde::{Deserialize, Serialize};
use std::collections::BTreeMap;
use wry::cookie::Cookie;

#[derive(Clone, PartialEq, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct Metadata {
    pub account_index: u8,
    #[serde(default)]
    pub on_behalf_of_user: String,
}

impl Metadata {
    pub fn parse(raw: &str) -> Option<Self> {
        let m: Self = serde_json::from_str(raw).ok()?;
        (m.account_index <= 99 && valid_delegate(&m.on_behalf_of_user)).then_some(m)
    }
}

fn valid_delegate(value: &str) -> bool {
    value.len() <= 128
        && value
            .bytes()
            .all(|b| b.is_ascii_alphanumeric() || b == b'_' || b == b'-')
}

#[derive(Serialize)]
#[serde(rename_all = "camelCase")]
pub struct Session {
    pub cookies: Vec<SessionCookie>,
    pub account_index: u8,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub on_behalf_of_user: String,
}

#[derive(Serialize)]
#[serde(rename_all = "camelCase")]
pub struct SessionCookie {
    name: String,
    value: String,
    domain: &'static str,
    path: &'static str,
    secure: bool,
    #[serde(skip_serializing_if = "Option::is_none")]
    expiration_date: Option<i64>,
}

fn allowed_name(name: &str) -> bool {
    matches!(
        name,
        "SAPISID"
            | "APISID"
            | "SID"
            | "HSID"
            | "SSID"
            | "LOGIN_INFO"
            | "SIDCC"
            | "__Secure-1PAPISID"
            | "__Secure-3PAPISID"
            | "__Secure-1PSID"
            | "__Secure-3PSID"
            | "__Secure-1PSIDTS"
            | "__Secure-3PSIDTS"
            | "__Secure-1PSIDCC"
            | "__Secure-3PSIDCC"
            | "PREF"
            | "VISITOR_INFO1_LIVE"
            | "VISITOR_PRIVACY_METADATA"
            | "YSC"
            | "CONSENT"
            | "SOCS"
    )
}

fn valid_value(value: &str) -> bool {
    !value.is_empty()
        && value
            .bytes()
            .all(|b| (0x21..=0x7e).contains(&b) && !matches!(b, b'"' | b',' | b';' | b'\\'))
}

// cookie::Cookie::domain removes one leading dot. It does not preserve the
// browser's hostOnly bit; accept only its exact canonical youtube.com domain and
// export Go's canonical form. The parent independently validates the session.
pub fn select(cookies: Vec<Cookie<'static>>, now: i64) -> Result<Option<Vec<SessionCookie>>, ()> {
    let mut selected: BTreeMap<String, SessionCookie> = BTreeMap::new();
    let mut bytes = 0;
    for c in cookies {
        if !allowed_name(c.name()) || c.domain() != Some("youtube.com") || c.path() != Some("/") {
            continue;
        }
        let expires = c.expires_datetime().map(|v| v.unix_timestamp());
        if expires.is_some_and(|e| e <= now) {
            continue;
        }
        if !valid_value(c.value()) || expires.is_some_and(|e| e > 253402300799) {
            return Err(());
        }
        if let Some(existing) = selected.get(c.name()) {
            if existing.value != c.value() {
                return Err(());
            }
            continue;
        }
        bytes += c.name().len() + c.value().len();
        if bytes > 64 * 1024 {
            return Err(());
        }
        selected.insert(
            c.name().into(),
            SessionCookie {
                name: c.name().into(),
                value: c.value().into(),
                domain: ".youtube.com",
                path: "/",
                secure: c.secure().unwrap_or(false),
                expiration_date: expires,
            },
        );
    }
    if !selected.contains_key("SAPISID") {
        return Ok(None);
    }
    Ok(Some(selected.into_values().collect()))
}

#[cfg(test)]
mod tests {
    use super::*;
    fn cookie(name: &str, domain: &str, path: &str, value: &str) -> Cookie<'static> {
        Cookie::build((name.to_owned(), value.to_owned()))
            .domain(domain.to_owned())
            .path(path.to_owned())
            .secure(true)
            .build()
    }

    #[test]
    fn missing_or_invalid_account_index_is_never_defaulted() {
        for raw in [
            "null",
            "{}",
            r#"{"accountIndex":100}"#,
            r#"{"accountIndex":-1}"#,
            r#"{"accountIndex":"0"}"#,
            r#"{"accountIndex":0,"onBehalfOfUser":"bad/channel"}"#,
        ] {
            assert!(Metadata::parse(raw).is_none());
        }
        assert_eq!(
            Metadata::parse(r#"{"accountIndex":3,"onBehalfOfUser":"brand_12"}"#)
                .unwrap()
                .account_index,
            3
        );
    }

    #[test]
    fn only_allowlisted_youtube_root_cookies_are_exported() {
        let selected = select(
            vec![
                cookie("SAPISID", ".youtube.com", "/", "synthetic"),
                cookie("SID", ".google.com", "/", "excluded"),
                cookie("APISID", "music.youtube.com", "/", "excluded"),
                cookie("SSID", "..youtube.com", "/", "excluded"),
                cookie("unrelated", ".youtube.com", "/", "excluded"),
                cookie("SID", ".youtube.com", "/other", "excluded"),
            ],
            10,
        )
        .unwrap()
        .unwrap();
        assert_eq!(selected.len(), 1);
        assert_eq!(selected[0].domain, ".youtube.com");
    }

    #[test]
    fn expired_required_cookie_conflicts_and_oversized_values_do_not_make_a_session() {
        let mut expired = cookie("SAPISID", ".youtube.com", "/", "synthetic");
        expired.set_expires(wry::cookie::time::OffsetDateTime::from_unix_timestamp(5).unwrap());
        assert!(select(vec![expired], 10).unwrap().is_none());
        assert!(
            select(
                vec![
                    cookie("SAPISID", ".youtube.com", "/", "a"),
                    cookie("SAPISID", ".youtube.com", "/", "b")
                ],
                10
            )
            .is_err()
        );
        assert!(
            select(
                vec![cookie("SAPISID", ".youtube.com", "/", &"x".repeat(65536))],
                10
            )
            .is_err()
        );
        assert!(
            select(
                vec![cookie("SAPISID", ".youtube.com", "/", "bad;value")],
                10
            )
            .is_err()
        );
    }
}

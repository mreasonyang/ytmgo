use std::io::{self, BufRead, Write};

use serde::{Deserialize, Serialize};

use crate::session::Session;

pub const MAX_OUTPUT: usize = 128 * 1024;
pub const MAX_INPUT: usize = 4096;

#[derive(Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct Request {
    pub version: u32,
    pub title: String,
    pub timeout_seconds: u64,
}

impl Request {
    pub fn valid(&self) -> bool {
        self.version == 1
            && !self.title.is_empty()
            && self.title.len() <= 128
            && !self.title.chars().any(char::is_control)
            && (1..=1800).contains(&self.timeout_seconds)
    }
}

#[derive(Deserialize)]
#[serde(deny_unknown_fields)]
pub struct Command {
    #[serde(rename = "command")]
    pub kind: CommandKind,
}

#[derive(Deserialize)]
#[serde(rename_all = "lowercase")]
pub enum CommandKind {
    Focus,
    Cancel,
    #[cfg(feature = "fixture")]
    #[serde(rename = "fixture-complete")]
    FixtureComplete,
}

#[derive(Clone, Copy, Serialize)]
#[serde(rename_all = "snake_case")]
#[allow(dead_code)] // Runtime errors are only produced on platforms needing an installed runtime.
pub enum Failure {
    RuntimeMissing,
    RuntimeOutdated,
    LoginTimeout,
    LoginFailed,
}

#[derive(Serialize)]
#[serde(tag = "status", rename_all = "lowercase")]
pub enum Outcome {
    Session { version: u32, session: Session },
    Cancelled { version: u32 },
    Error { version: u32, error: Failure },
}

impl Outcome {
    pub fn session(session: Session) -> Self {
        Self::Session {
            version: 1,
            session,
        }
    }
    pub fn cancelled() -> Self {
        Self::Cancelled { version: 1 }
    }
    pub fn error(error: Failure) -> Self {
        Self::Error { version: 1, error }
    }
}

// Bound input before allocation: the parent is the only protocol peer.
pub fn read_line(reader: &mut impl BufRead) -> io::Result<Option<Vec<u8>>> {
    let mut line = Vec::new();
    loop {
        let buf = reader.fill_buf()?;
        if buf.is_empty() {
            return if line.is_empty() {
                Ok(None)
            } else {
                Err(io::ErrorKind::UnexpectedEof.into())
            };
        }
        let count = buf
            .iter()
            .position(|b| *b == b'\n')
            .map_or(buf.len(), |n| n + 1);
        if line.len() + count > MAX_INPUT {
            return Err(io::ErrorKind::InvalidData.into());
        }
        let complete = buf[count - 1] == b'\n';
        line.extend_from_slice(&buf[..count]);
        reader.consume(count);
        if complete {
            return Ok(Some(line));
        }
    }
}

// Serialize before writing so a size failure cannot leave a partial session on stdout.
pub fn write_outcome(outcome: &Outcome, writer: &mut impl Write) -> io::Result<()> {
    let mut encoded = serde_json::to_vec(outcome).map_err(|_| io::ErrorKind::InvalidData)?;
    if encoded.len() + 1 > MAX_OUTPUT {
        encoded = serde_json::to_vec(&Outcome::error(Failure::LoginFailed))
            .map_err(|_| io::ErrorKind::InvalidData)?;
    }
    encoded.push(b'\n');
    writer.write_all(&encoded)?;
    writer.flush()
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::io::{BufReader, Cursor};

    #[test]
    fn oversized_or_partial_input_is_rejected_without_consuming_next_command() {
        assert!(read_line(&mut Cursor::new(vec![b'x'; MAX_INPUT + 1])).is_err());
        assert!(read_line(&mut Cursor::new(b"{\"command\":\"cancel\"}")).is_err());
        let mut r = BufReader::with_capacity(3, Cursor::new(b"first\nsecond\n"));
        assert_eq!(read_line(&mut r).unwrap().unwrap(), b"first\n");
        assert_eq!(read_line(&mut r).unwrap().unwrap(), b"second\n");
        assert!(read_line(&mut r).unwrap().is_none());
    }

    #[test]
    fn protocol_refuses_unbounded_timeout_unknown_fields_and_control_titles() {
        let r: Request =
            serde_json::from_str(r#"{"version":1,"title":"Login","timeoutSeconds":600}"#).unwrap();
        assert!(r.valid());
        assert!(
            !Request {
                timeout_seconds: 1801,
                ..r
            }
            .valid()
        );
        assert!(
            serde_json::from_str::<Request>(
                r#"{"version":1,"title":"x","timeoutSeconds":600,"url":"https://example.com"}"#
            )
            .is_err()
        );
        assert!(serde_json::from_str::<Command>(r#"{"command":"focus","url":"x"}"#).is_err());
        #[cfg(not(feature = "fixture"))]
        assert!(serde_json::from_str::<Command>(r#"{"command":"fixture-complete"}"#).is_err());
        #[cfg(feature = "fixture")]
        assert!(matches!(
            serde_json::from_str::<Command>(r#"{"command":"fixture-complete"}"#).map(|c| c.kind),
            Ok(CommandKind::FixtureComplete)
        ));
        assert!(
            !Request {
                version: 1,
                title: "x\ny".into(),
                timeout_seconds: 600
            }
            .valid()
        );
    }
}

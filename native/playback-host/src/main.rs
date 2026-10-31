#![cfg_attr(windows, windows_subsystem = "windows")]
// Private, bounded pipes only. No cookies, HTTP client, filesystem API, Node
// modules, media playback, or logging is exposed to evaluated JavaScript.
mod dom;
#[cfg(windows)]
mod windows_profile;
use serde::Deserialize;
use serde_json::{Value, json};
use std::io::{self, BufRead, Read, Write};
use std::time::{Duration, Instant};

const INPUT_LIMIT: usize = 24 * 1024 * 1024;
const OUTPUT_LIMIT: usize = 9 * 1024 * 1024;
const EXTRACTOR: &str = include_str!("../assets/extractor.js");
const PROOF: &str = include_str!("../assets/proof.js");

#[derive(Deserialize)]
#[serde(deny_unknown_fields)]
struct Request {
    id: u64,
    code: String,
}

fn read_request(input: &mut impl BufRead) -> Result<Option<Request>, ()> {
    let mut line = Vec::new();
    input
        .take((INPUT_LIMIT + 1) as u64)
        .read_until(b'\n', &mut line)
        .map_err(|_| ())?;
    if line.is_empty() {
        return Ok(None);
    }
    if line.len() > INPUT_LIMIT || line.last() != Some(&b'\n') {
        return Err(());
    }
    let value: Request = serde_json::from_slice(&line).map_err(|_| ())?;
    if value.id == 0 || value.id > 9007199254740991 || value.code.len() > 20 * 1024 * 1024 {
        return Err(());
    }
    Ok(Some(value))
}

fn write_reply(value: &Value) -> Result<(), ()> {
    let mut bytes = serde_json::to_vec(value).map_err(|_| ())?;
    if bytes.len() > OUTPUT_LIMIT {
        return Err(());
    }
    bytes.push(b'\n');
    let mut output = io::stdout().lock();
    output
        .write_all(&bytes)
        .and_then(|_| output.flush())
        .map_err(|_| ())
}

fn evaluate(code: &str, extract: bool) -> Result<Value, &'static str> {
    let runtime = rquickjs::Runtime::new().map_err(|_| "script_failed")?;
    runtime.set_memory_limit(if extract { 256 << 20 } else { 32 << 20 });
    runtime.set_max_stack_size(2 << 20);
    let deadline = Instant::now() + Duration::from_secs(if extract { 25 } else { 3 });
    runtime.set_interrupt_handler(Some(Box::new(move || Instant::now() > deadline)));
    let context = rquickjs::Context::full(&runtime).map_err(|_| "script_failed")?;
    // A fresh realm per operation: dynamic scripts cannot alter the next call.
    context.with(|ctx| {
        ctx.eval::<(), _>(
            "globalThis.console = Object.freeze({log(){},warn(){},error(){},info(){},debug(){}});",
        )
        .map_err(|_| "script_failed")?;
        if extract {
            ctx.eval::<(), _>(EXTRACTOR).map_err(|_| "script_failed")?;
        }
        let result: String = ctx
            .eval(format!("JSON.stringify(({}))", code))
            .map_err(|error| {
                if matches!(error, rquickjs::Error::Allocation) {
                    return "script_memory_limit";
                }
                if matches!(error, rquickjs::Error::Utf8(_)) {
                    return "script_encoding_failed";
                }
                if matches!(error, rquickjs::Error::FromJs { .. }) {
                    return "script_result_type";
                }
                let caught = rquickjs::CaughtError::from_error(&ctx, error);
                if Instant::now() > deadline {
                    return "script_timeout";
                }
                if let rquickjs::CaughtError::Exception(ref exception) = caught {
                    match exception.message().as_deref() {
                        Some("out of memory") => return "script_memory_limit",
                        Some("stack overflow") => return "script_stack_limit",
                        Some("player_timestamp_missing") => return "player_timestamp_missing",
                        Some("player_function_missing") => return "player_function_missing",
                        Some("player_output_limit") => return "script_output_limit",
                        _ => {}
                    }
                    match exception
                        .as_object()
                        .get::<_, String>("name")
                        .ok()
                        .as_deref()
                    {
                        Some("ReferenceError") => return "script_reference_error",
                        Some("TypeError") => return "script_type_error",
                        Some("SyntaxError") => return "script_syntax_error",
                        Some("InternalError") => return "script_internal_error",
                        Some("RangeError") => return "script_range_error",
                        Some("Error") => return "script_error",
                        _ => {}
                    }
                }
                if matches!(caught, rquickjs::CaughtError::Value(_)) {
                    "script_exception_value"
                } else {
                    "script_conversion_failed"
                }
            })?;
        if result.len() > OUTPUT_LIMIT {
            return Err("script_output_limit");
        }
        serde_json::from_str(&result).map_err(|_| "script_json_failed")
    })
}

fn run() -> Result<(), ()> {
    // The only argument is a public execution-mode enum, never user data.
    let mode = std::env::args().nth(1).ok_or(())?;
    if mode == "check" {
        return if evaluate("1+1", false).map_err(|_| ())? == json!(2) {
            Ok(())
        } else {
            Err(())
        };
    }
    if mode == "dom" {
        return dom::run();
    }
    if mode != "extract" && mode != "eval" {
        return Err(());
    }
    let mut input = io::BufReader::new(io::stdin());
    let mut last = 0;
    while let Some(request) = read_request(&mut input)? {
        if request.id <= last {
            return Err(());
        }
        last = request.id;
        let reply = match evaluate(&request.code, mode == "extract") {
            Ok(value) => json!({"id": request.id, "ok": true, "value": value}),
            Err(category) => json!({"id": request.id, "ok": false, "category": category}),
        };
        write_reply(&reply)?;
        if mode == "extract" {
            break;
        }
    }
    Ok(())
}

fn main() {
    std::panic::set_hook(Box::new(|_| {}));
    if !matches!(std::panic::catch_unwind(run), Ok(Ok(()))) {
        std::process::exit(2)
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn bounded_protocol_and_fresh_realm() {
        assert!(read_request(&mut &b"{\"id\":0,\"code\":\"1\"}\n"[..]).is_err());
        assert!(read_request(&mut &b"{\"id\":1,\"code\":\"1\",\"extra\":0}\n"[..]).is_err());
        assert!(read_request(&mut &b"{\"id\":1,\"code\":\"1\"}"[..]).is_err());
        assert_eq!(
            evaluate("(()=>{globalThis.changed=4;return 3})()", false).unwrap(),
            json!(3)
        );
        assert_eq!(
            evaluate("typeof changed", false).unwrap(),
            json!("undefined")
        );
        assert_eq!(
            evaluate("[typeof fetch,typeof process,typeof require]", false).unwrap(),
            json!(["undefined", "undefined", "undefined"])
        );
        assert!(evaluate("(()=>{throw new Error('private')})()", false).is_err());
    }
    #[test]
    fn interpreter_interrupts_nonterminating_script() {
        let began = Instant::now();
        assert!(evaluate("(()=>{while(true){} })()", false).is_err());
        assert!(began.elapsed() < Duration::from_secs(8));
    }
}

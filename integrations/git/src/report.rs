use crate::host;
use std::collections::HashMap;

pub struct InputRequest {
    pub key: String,
    pub prompt: String,
    pub masked: bool,
}

#[derive(Default)]
pub struct StateReport {
    pub present: bool,
    pub reachable: bool,
    pub binary_path: String,
    pub in_path: bool,
    pub manager: String,
    pub error: String,
    pub observations: Vec<String>,
    pub exports: HashMap<String, String>,
    pub needs_input: Vec<InputRequest>,
}

pub fn write_state(r: &StateReport) {
    let mut buf = format!(
        r#"{{"present":{},"reachable":{},"in_path":{},"manager":{}"#,
        r.present,
        r.reachable,
        r.in_path,
        host::json_str(&r.manager),
    );
    if !r.binary_path.is_empty() {
        buf.push_str(&format!(r#","binary_path":{}"#, host::json_str(&r.binary_path)));
    }
    if !r.error.is_empty() {
        buf.push_str(&format!(r#","error":{}"#, host::json_str(&r.error)));
    }
    if !r.observations.is_empty() {
        buf.push_str(r#","observations":["#);
        for (i, o) in r.observations.iter().enumerate() {
            if i > 0 {
                buf.push(',');
            }
            buf.push_str(&host::json_str(o));
        }
        buf.push(']');
    }
    if !r.exports.is_empty() {
        buf.push_str(r#","exports":{"#);
        let mut first = true;
        for (k, v) in &r.exports {
            if !first {
                buf.push(',');
            }
            first = false;
            buf.push_str(&host::json_str(k));
            buf.push(':');
            buf.push_str(&host::json_str(v));
        }
        buf.push('}');
    }
    if !r.needs_input.is_empty() {
        buf.push_str(r#","needs_input":["#);
        for (i, req) in r.needs_input.iter().enumerate() {
            if i > 0 {
                buf.push(',');
            }
            buf.push_str(&format!(
                r#"{{"key":{},"prompt":{},"masked":{}}}"#,
                host::json_str(&req.key),
                host::json_str(&req.prompt),
                req.masked,
            ));
        }
        buf.push(']');
    }
    buf.push('}');
    host::write_output(buf.as_bytes());
}

pub fn write_detect(detected: bool, role: &str, brand: &str, version: &str) {
    if !detected {
        host::write_output(b"{\"detected\":false}");
        return;
    }
    let buf = format!(
        r#"{{"detected":true,"resources":[{{"role":{},"brand":{},"version":{}}}]}}"#,
        host::json_str(role),
        host::json_str(brand),
        host::json_str(version),
    );
    host::write_output(buf.as_bytes());
}

pub fn write_error(msg: &str) {
    let buf = format!(
        r#"{{"present":false,"reachable":false,"in_path":false,"manager":"","error":{}}}"#,
        host::json_str(msg),
    );
    host::write_output(buf.as_bytes());
}

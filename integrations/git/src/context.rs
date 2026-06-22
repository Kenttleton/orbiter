use std::collections::HashMap;

pub struct Platform {
    pub os: String,           // "darwin" | "linux" | "windows"
    #[allow(dead_code)]
    pub arch: String,         // "amd64" | "arm64"
}

pub struct SelfEntity {
    pub role: String,
    #[allow(dead_code)]
    pub brand: String,
    pub config: String, // raw JSON object string for this entity's config
}

pub struct RemoteResource {
    pub url: String, // from Resources["remote"][0].resource.config["url"]
}

pub struct ResolvedContext {
    pub platform: Platform,
    pub self_entity: SelfEntity,
    pub remote: Option<RemoteResource>,
    pub has_agent_transponder: bool,    // true if Transponders["agent"] is non-empty
    pub has_keychain_transponder: bool, // true if Transponders["keychain"] is non-empty
    pub responses: HashMap<String, String>,
    pub binaries: HashMap<String, String>,
}

// Parse a ResolvedContext from raw JSON bytes.
pub fn parse(input: &[u8]) -> Result<ResolvedContext, String> {
    let s = core::str::from_utf8(input).map_err(|_| "input is not valid UTF-8".to_string())?;

    let platform = Platform {
        os: extract_nested_str(s, "platform", "os").unwrap_or_default(),
        arch: extract_nested_str(s, "platform", "arch").unwrap_or_default(),
    };

    let self_role = extract_nested_str(s, "self", "role").unwrap_or_default();
    let self_brand = extract_nested_str(s, "self", "brand").unwrap_or_default();
    let self_config = extract_nested_str(s, "self", "config").unwrap_or_default();

    let self_entity = SelfEntity {
        role: self_role,
        brand: self_brand,
        config: self_config,
    };

    let remote = parse_remote(s);

    let has_agent_transponder = transponder_non_empty(s, "agent");
    let has_keychain_transponder = transponder_non_empty(s, "keychain");

    let responses = parse_responses(s);
    let binaries = parse_binaries(s);

    Ok(ResolvedContext {
        platform,
        self_entity,
        remote,
        has_agent_transponder,
        has_keychain_transponder,
        responses,
        binaries,
    })
}

// Check whether the detect input contains ".git/config" as a key (for detect handler).
pub fn has_git_config(input: &[u8]) -> bool {
    let needle = b"\".git/config\"";
    input.windows(needle.len()).any(|w| w == needle)
}

// Extract a string value for `key` from a JSON object string.
// Returns None if the key is absent or the value is not a string.
pub fn config_str(config: &str, key: &str) -> Option<String> {
    extract_str_field(config, key)
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

/// Extract a quoted string value for `needle_key` from `s`.
/// Looks for the pattern `"<key>":"<value>"` (ignoring whitespace between `:` and `"`).
fn extract_str_field(s: &str, key: &str) -> Option<String> {
    let needle = format!("\"{}\"", key);
    let key_pos = s.find(&needle)?;
    let after_key = &s[key_pos + needle.len()..];
    // Skip optional whitespace then ':'
    let colon_pos = after_key.find(':')?;
    let after_colon = &after_key[colon_pos + 1..];
    // Skip optional whitespace then expect '"'
    let quote_start = after_colon.find('"')?;
    let value_start = quote_start + 1;
    let rest = &after_colon[value_start..];
    Some(read_json_string(rest))
}

/// Read characters from `s` (which starts just after the opening `"`) until
/// the closing unescaped `"`, interpreting JSON escape sequences.
fn read_json_string(s: &str) -> String {
    let mut out = String::new();
    let mut chars = s.chars();
    loop {
        match chars.next() {
            None | Some('"') => break,
            Some('\\') => match chars.next() {
                Some('"') => out.push('"'),
                Some('\\') => out.push('\\'),
                Some('/') => out.push('/'),
                Some('n') => out.push('\n'),
                Some('r') => out.push('\r'),
                Some('t') => out.push('\t'),
                Some('u') => {
                    // Read 4 hex digits
                    let hex: String = chars.by_ref().take(4).collect();
                    if let Ok(n) = u32::from_str_radix(&hex, 16) {
                        if let Some(c) = char::from_u32(n) {
                            out.push(c);
                        }
                    }
                }
                Some(c) => out.push(c),
                None => break,
            },
            Some(c) => out.push(c),
        }
    }
    out
}

/// Extract a string field nested one level deep: `"<outer>": { ... "<inner>": "<value>" ... }`.
/// This is a best-effort search: find the outer key, then search within the following
/// object literal for the inner key.
fn extract_nested_str(s: &str, outer: &str, inner: &str) -> Option<String> {
    let outer_needle = format!("\"{}\"", outer);
    let outer_pos = s.find(&outer_needle)?;
    let after_outer = &s[outer_pos + outer_needle.len()..];
    // Find the opening brace of the nested object
    let brace_pos = after_outer.find('{')?;
    // Find the matching closing brace (simple depth tracking)
    let obj_start = brace_pos + 1;
    let inner_obj = &after_outer[obj_start..];
    let close = find_object_end(inner_obj)?;
    let obj_body = &inner_obj[..close];
    extract_str_field(obj_body, inner)
}

/// Given a string that starts just inside `{`, find the index of the matching `}`.
/// Handles nested braces and brackets, and skips string literals.
/// Brace depth and bracket depth are tracked separately so that `]` never
/// closes a `{`, preventing incorrect slice boundaries when arrays appear
/// inside objects.
fn find_object_end(s: &str) -> Option<usize> {
    let mut brace_depth = 1usize;
    let mut bracket_depth = 0usize;
    let mut chars = s.char_indices();
    loop {
        match chars.next() {
            None => return None,
            Some((_, '{')) => brace_depth += 1,
            Some((_, '[')) => bracket_depth += 1,
            Some((_, ']')) => {
                if bracket_depth > 0 {
                    bracket_depth -= 1;
                }
            }
            Some((i, '}')) => {
                brace_depth -= 1;
                if brace_depth == 0 {
                    return Some(i);
                }
            }
            Some((_, '"')) => {
                // Skip string literal
                loop {
                    match chars.next() {
                        None => return None,
                        Some((_, '"')) => break,
                        Some((_, '\\')) => {
                            chars.next(); // skip escaped char
                        }
                        _ => {}
                    }
                }
            }
            _ => {}
        }
    }
}

/// Extract URL from `resources.remote[0].resource.config["url"]`.
///
/// The JSON shape is:
/// ```json
/// "resources": { "remote": [ { "resource": { "config": "{\"url\":\"git@...\"}" } } ] }
/// ```
/// `config` is a JSON-encoded string, so we parse the outer JSON to get the
/// raw config string, then parse that inner JSON string for "url".
fn parse_remote(s: &str) -> Option<RemoteResource> {
    // Find "remote" inside the resources object
    let resources_needle = "\"resources\"";
    let res_pos = s.find(resources_needle)?;
    let after_res = &s[res_pos + resources_needle.len()..];
    let brace = after_res.find('{')?;
    let inner = &after_res[brace + 1..];
    let close = find_object_end(inner)?;
    let resources_body = &inner[..close];

    // Within resources, find "remote" array
    let remote_needle = "\"remote\"";
    let remote_pos = resources_body.find(remote_needle)?;
    let after_remote = &resources_body[remote_pos + remote_needle.len()..];
    let bracket = after_remote.find('[')?;
    let arr_inner = &after_remote[bracket + 1..];
    // Skip optional whitespace/newlines to first '{'
    let first_obj_start = arr_inner.find('{')?;
    let first_obj = &arr_inner[first_obj_start + 1..];
    let first_obj_end = find_object_end(first_obj)?;
    let first_obj_body = &first_obj[..first_obj_end];

    // Within first array element, find "resource"
    let resource_needle = "\"resource\"";
    let resource_pos = first_obj_body.find(resource_needle)?;
    let after_resource = &first_obj_body[resource_pos + resource_needle.len()..];
    let inner_brace = after_resource.find('{')?;
    let resource_inner = &after_resource[inner_brace + 1..];
    let resource_end = find_object_end(resource_inner)?;
    let resource_body = &resource_inner[..resource_end];

    // config field is a JSON-encoded string — extract it
    let config_raw = extract_str_field(resource_body, "config")?;

    // Now parse the config string itself for "url"
    let url = config_str(&config_raw, "url")?;
    if url.is_empty() {
        return None;
    }
    Some(RemoteResource { url })
}

/// Return true if `transponders.<name>` is a non-empty JSON array.
fn transponder_non_empty(s: &str, name: &str) -> bool {
    let transponders_needle = "\"transponders\"";
    let tp_pos = match s.find(transponders_needle) {
        Some(p) => p,
        None => return false,
    };
    let after_tp = &s[tp_pos + transponders_needle.len()..];
    let brace = match after_tp.find('{') {
        Some(b) => b,
        None => return false,
    };
    let inner = &after_tp[brace + 1..];
    let close = match find_object_end(inner) {
        Some(c) => c,
        None => return false,
    };
    let tp_body = &inner[..close];

    // Find the named key within the transponders object
    let key_needle = format!("\"{}\"", name);
    let key_pos = match tp_body.find(&key_needle) {
        Some(p) => p,
        None => return false,
    };
    let after_key = &tp_body[key_pos + key_needle.len()..];
    let bracket = match after_key.find('[') {
        Some(b) => b,
        None => return false,
    };
    // Check that there is a non-whitespace, non-`]` character before the closing bracket
    let arr_content = &after_key[bracket + 1..];
    arr_content.find(|c: char| !c.is_whitespace() && c != ']').is_some()
}

/// Parse all key-value string pairs from `responses`.
fn parse_responses(s: &str) -> HashMap<String, String> {
    let mut map = HashMap::new();

    let responses_needle = "\"responses\"";
    let res_pos = match s.find(responses_needle) {
        Some(p) => p,
        None => return map,
    };
    let after_res = &s[res_pos + responses_needle.len()..];
    let brace = match after_res.find('{') {
        Some(b) => b,
        None => return map,
    };
    let inner = &after_res[brace + 1..];
    let close = match find_object_end(inner) {
        Some(c) => c,
        None => return map,
    };
    let body = &inner[..close];

    // Iterate over `"key": "value"` pairs in this flat object
    let mut remaining = body;
    loop {
        // Find next key opening quote
        let q = match remaining.find('"') {
            Some(p) => p,
            None => break,
        };
        let key_start = q + 1;
        let key_rest = &remaining[key_start..];
        let key = read_json_string(key_rest);
        // Advance past the key string (find the closing `"` ourselves)
        let key_end = find_string_end(&remaining[key_start..]);
        remaining = &remaining[key_start + key_end..];

        // Find ':'
        let colon = match remaining.find(':') {
            Some(p) => p,
            None => break,
        };
        remaining = &remaining[colon + 1..];

        // Find value opening quote; skip if value is not a string
        let vq = match remaining.find('"') {
            Some(p) => p,
            None => break,
        };
        // Check there is no '{', '[' before the quote (would mean non-string value)
        let before_quote = &remaining[..vq];
        if before_quote.contains('{') || before_quote.contains('[') {
            // Non-string value — skip the entire value using depth tracking so
            // that commas inside nested objects/arrays are not mistaken for
            // entry separators.
            let val_slice = remaining.trim_start();
            let skip_len = skip_json_value(val_slice);
            // Advance past the value, then past any trailing ','
            let after_val = &remaining[remaining.len() - val_slice.len() + skip_len..];
            let trimmed = after_val.trim_start();
            if trimmed.starts_with(',') {
                remaining = &trimmed[1..];
            } else {
                remaining = trimmed;
            }
            continue;
        }
        let val_start = vq + 1;
        let val_rest = &remaining[val_start..];
        let value = read_json_string(val_rest);
        let val_end = find_string_end(&remaining[val_start..]);
        remaining = &remaining[val_start + val_end..];

        if !key.is_empty() {
            map.insert(key, value);
        }
    }
    map
}

/// Skip a complete JSON value that starts at the beginning of `s`.
/// Works for objects `{…}`, arrays `[…]`, strings `"…"`, and primitives
/// (numbers, booleans, null) that are terminated by `,` or `}`.
/// Returns the byte length consumed.
fn skip_json_value(s: &str) -> usize {
    let mut chars = s.char_indices();
    // Peek at the first non-whitespace character to decide the value kind.
    let (first_idx, first_char) = loop {
        match chars.next() {
            None => return s.len(),
            Some((i, c)) if !c.is_whitespace() => break (i, c),
            _ => {}
        }
    };

    match first_char {
        '{' | '[' => {
            // Depth-tracked scan; strings inside the structure are skipped
            // so their braces/brackets don't affect depth.
            let mut depth = 1usize;
            loop {
                match chars.next() {
                    None => return s.len(),
                    Some((_, '"')) => {
                        // Skip string literal contents
                        loop {
                            match chars.next() {
                                None => return s.len(),
                                Some((_, '"')) => break,
                                Some((_, '\\')) => { chars.next(); }
                                _ => {}
                            }
                        }
                    }
                    Some((_, '{')) | Some((_, '[')) => depth += 1,
                    Some((i, '}')) | Some((i, ']')) => {
                        depth -= 1;
                        if depth == 0 {
                            return i + 1;
                        }
                    }
                    _ => {}
                }
            }
        }
        '"' => {
            // String value — first_idx points at the `"`, content starts after.
            let rest = &s[first_idx + 1..];
            let end = find_string_end(rest);
            first_idx + 1 + end
        }
        _ => {
            // Primitive: scan until ',' or '}' or end
            for (i, c) in s[first_idx..].char_indices() {
                if c == ',' || c == '}' {
                    return first_idx + i;
                }
            }
            s.len()
        }
    }
}

/// Given a string starting just after the opening `"` of a JSON string,
/// return the byte index of the character *after* the closing `"`.
fn find_string_end(s: &str) -> usize {
    let mut chars = s.char_indices();
    loop {
        match chars.next() {
            None => return s.len(),
            Some((i, '"')) => return i + 1,
            Some((_, '\\')) => {
                chars.next(); // skip escaped char
            }
            _ => {}
        }
    }
}

/// Parse binaries map from "binaries": {...} object.
fn parse_binaries(s: &str) -> HashMap<String, String> {
    let mut map = HashMap::new();

    let binaries_needle = "\"binaries\"";
    let bin_pos = match s.find(binaries_needle) {
        Some(p) => p,
        None => return map,
    };
    let after_bin = &s[bin_pos + binaries_needle.len()..];
    let brace = match after_bin.find('{') {
        Some(b) => b,
        None => return map,
    };
    let inner = &after_bin[brace + 1..];
    let close = match find_object_end(inner) {
        Some(c) => c,
        None => return map,
    };

    let binaries_body = &inner[..close];
    let mut remaining = binaries_body;

    while let Some(quote_pos) = remaining.find('"') {
        let after_quote = &remaining[quote_pos + 1..];
        if let Some(close_quote) = after_quote.find('"') {
            let key = after_quote[..close_quote].to_string();
            let after_key = &after_quote[close_quote + 1..];

            if let Some(colon_pos) = after_key.find(':') {
                let after_colon = &after_key[colon_pos + 1..];
                if let Some(val_quote) = after_colon.find('"') {
                    let val_str = &after_colon[val_quote + 1..];
                    if let Some(close_val) = val_str.find('"') {
                        let value = val_str[..close_val].to_string();
                        map.insert(key, value);
                        remaining = &val_str[close_val + 1..];
                        continue;
                    }
                }
            }
        }
        break;
    }

    map
}

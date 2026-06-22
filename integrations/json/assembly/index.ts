import { JSON } from "assemblyscript-json/assembly";

@external("orbiter", "read_input")
declare function read_input(ptr: i32, max: i32): i32;

@external("orbiter", "write_output")
declare function write_output(ptr: i32, len: i32): void;

const BUF_SIZE: i32 = 131072; // 128 KB — larger than default for JSON payloads

function readInput(): Uint8Array {
  const buf = new Uint8Array(BUF_SIZE);
  const n = read_input(buf.dataStart, buf.byteLength);
  return buf.slice(0, n);
}

function writeStr(s: string): void {
  const encoded = String.UTF8.encode(s, false);
  const buf = Uint8Array.wrap(encoded);
  write_output(buf.dataStart, buf.byteLength);
}

// extractPath parses path out of a config JSON string like {"path":"/output/context.json"}.
function extractPath(configStr: string): string {
  const parsed = JSON.parse(configStr);
  if (!parsed.isObj) return "";
  const cfg = parsed as JSON.Obj;
  const pathVal = cfg.get("path");
  if (pathVal == null || !pathVal.isString) return "";
  return (pathVal as JSON.Str).valueOf();
}

export function detect(): void {
  writeStr('{"detected":false}');
}

export function initialize(): void {
  calibrate();
}

export function scan(): void {
  calibrate();
}

export function calibrate(): void {
  const inputBytes = readInput();
  const inputStr = String.UTF8.decode(inputBytes.buffer);

  const parsed = JSON.parse(inputStr);
  if (!parsed.isObj) {
    writeStr('{"present":false,"reachable":false,"manager":"","error":"invalid input"}');
    return;
  }

  const ctx = parsed as JSON.Obj;
  let path = "";

  // Navigate: ctx.self.config -> parse -> .path
  const selfVal = ctx.get("self");
  if (selfVal != null && selfVal.isObj) {
    const self = selfVal as JSON.Obj;
    const configVal = self.get("config");
    if (configVal != null && configVal.isString) {
      path = extractPath((configVal as JSON.Str).valueOf());
    }
  }

  if (path.length == 0) {
    writeStr('{"present":false,"reachable":false,"manager":"","error":"no path in self config"}');
    return;
  }

  // Build StateReport with config.write_files: {path: inputStr}
  // ResolvedContext contains no resolved secret values — safe to export as-is.
  const writeFiles = new JSON.Obj();
  writeFiles.set(path, inputStr);

  const config = new JSON.Obj();
  config.set("write_files", writeFiles);

  const report = new JSON.Obj();
  report.set("present", true);
  report.set("reachable", true);
  report.set("config", config);

  writeStr(report.stringify());
}

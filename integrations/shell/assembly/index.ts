import { JSON } from "assemblyscript-json/assembly";

@external("orbiter", "read_input")
declare function read_input(ptr: i32, max: i32): i32;

@external("orbiter", "write_output")
declare function write_output(ptr: i32, len: i32): void;

const BUF_SIZE: i32 = 65536;

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

// env/shell is always detected — every shell environment has variables.
export function detect(): void {
  const out = new JSON.Obj();
  out.set("detected", true);
  const resources = new JSON.Arr();
  const resource = new JSON.Obj();
  resource.set("role", "env");
  resource.set("brand", "shell");
  resources.push(resource);
  out.set("resources", resources);
  writeStr(out.stringify());
}

// scan: shell environment is always present — discovery-based, no commands executed.
// `sh` is banned by the host banlist, so we report state without running any commands.
export function initialize(): void {
  readInput();
  const obj = new JSON.Obj();
  obj.set("present", true);
  obj.set("reachable", true);
  obj.set("in_path", false);
  obj.set("manager", "shell");
  writeStr(obj.stringify());
}

export function scan(): void {
  initialize();
}

// env transponders are read-only — calibrate is the same as scan
export function calibrate(): void {
  initialize();
}

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

export function detect(): void {
  readInput();
  const out = new JSON.Obj();
  out.set("detected", true);
  const resources = new JSON.Arr();
  const resource = new JSON.Obj();
  resource.set("role", "shell");
  resource.set("brand", "powershell");
  resources.push(resource);
  out.set("resources", resources);
  writeStr(out.stringify());
}

function report(): void {
  readInput();
  const obj = new JSON.Obj();
  obj.set("present", true);
  obj.set("reachable", true);
  obj.set("manager", "shell");
  writeStr(obj.stringify());
}

export function initialize(): void { report(); }
export function scan(): void { report(); }
export function calibrate(): void { report(); }

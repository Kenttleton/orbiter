import { JSON } from "assemblyscript-json/assembly";

@external("orbiter", "read_input")
declare function read_input(ptr: i32, max: i32): i32;

@external("orbiter", "write_output")
declare function write_output(ptr: i32, len: i32): void;

@external("orbiter", "run_command")
declare function run_command(specPtr: i32, specLen: i32, outPtr: i32, outMax: i32): i32;

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

function runCmd(cmd: string, args: string[]): string {
  const spec = new JSON.Obj();
  spec.set("cmd", cmd);
  const argsArr = new JSON.Arr();
  for (let i = 0; i < args.length; i++) {
    argsArr.push(new JSON.Str(args[i]));
  }
  spec.set("args", argsArr);
  const specStr = spec.stringify();
  const specEncoded = String.UTF8.encode(specStr, false);
  const specBuf = Uint8Array.wrap(specEncoded);
  const outBuf = new Uint8Array(BUF_SIZE);
  const n = run_command(specBuf.dataStart, specBuf.byteLength, outBuf.dataStart, outBuf.byteLength);
  return String.UTF8.decode(outBuf.buffer.slice(0, n)).trimEnd();
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

// scan: check which well-known env vars are set.
// The transponder doesn't know which vars a dependent resource needs —
// it reports overall shell env health.
export function initialize(): void {
  readInput();
  // Sample a few well-known vars to build an environment health picture
  const path = runCmd("sh", ["-c", "echo $PATH"]);
  const home = runCmd("sh", ["-c", "echo $HOME"]);
  const observations: string[] = [];
  if (path.length > 0) observations.push("PATH: set (" + path.length.toString() + " chars)");
  if (home.length > 0) observations.push("HOME: " + home);
  const obj = new JSON.Obj();
  obj.set("present", true);
  obj.set("reachable", true);
  obj.set("in_path", false);
  obj.set("manager", "shell");
  if (observations.length > 0) {
    const arr = new JSON.Arr();
    for (let i = 0; i < observations.length; i++) {
      arr.push(new JSON.Str(observations[i]));
    }
    obj.set("observations", arr);
  }
  writeStr(obj.stringify());
}

export function scan(): void {
  initialize();
}

// env transponders are read-only — calibrate is the same as scan
export function calibrate(): void {
  initialize();
}

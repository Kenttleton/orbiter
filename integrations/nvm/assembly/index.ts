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

function writeState(
  present: bool, reachable: bool, inPath: bool,
  binaryPath: string, manager: string, errMsg: string,
  observations: string[]
): void {
  const obj = new JSON.Obj();
  obj.set("present", present);
  obj.set("reachable", reachable);
  obj.set("in_path", inPath);
  obj.set("manager", manager);
  if (binaryPath.length > 0) obj.set("binary_path", binaryPath);
  if (errMsg.length > 0) obj.set("error", errMsg);
  if (observations.length > 0) {
    const arr = new JSON.Arr();
    for (let i = 0; i < observations.length; i++) {
      arr.push(new JSON.Str(observations[i]));
    }
    obj.set("observations", arr);
  }
  writeStr(obj.stringify());
}

export function detect(): void {
  const inputBytes = readInput();
  const inputStr = String.UTF8.decode(inputBytes.buffer);
  const parsed = JSON.parse(inputStr);
  if (!parsed.isObj) {
    writeStr('{"detected":false}');
    return;
  }
  const ctx = parsed as JSON.Obj;
  const filesVal = ctx.get("files");
  let hasNvmrc = false;
  let hasNodeVersion = false;
  if (filesVal != null && filesVal.isObj) {
    const files = filesVal as JSON.Obj;
    hasNvmrc = files.has(".nvmrc");
    hasNodeVersion = files.has(".node-version");
  }
  if (!hasNvmrc && !hasNodeVersion) {
    writeStr('{"detected":false}');
    return;
  }
  const out = new JSON.Obj();
  out.set("detected", true);
  const resources = new JSON.Arr();
  const resource = new JSON.Obj();
  resource.set("role", "manager");
  resource.set("brand", "nvm");
  resources.push(resource);
  out.set("resources", resources);
  writeStr(out.stringify());
}

export function initialize(): void {
  readInput();
  // nvm is a shell function, not a binary — we cannot reliably resolve NVM_DIR
  // from a WASM guest without calling `sh` (which is banned). Instead, report
  // nvm as present when detected (context had .nvmrc/.node-version). Report the
  // active node version as an observation if node is in PATH.
  const nodeVersion = runCmd("node", ["--version"]);
  const observations: string[] = ["manager: nvm"];
  if (nodeVersion.length > 0) {
    observations.push("active node: " + nodeVersion);
  }
  writeState(true, true, false, "", "nvm", "", observations);
}

export function scan(): void {
  initialize();
}

export function calibrate(): void {
  readInput();
  const nodeVersion = runCmd("node", ["--version"]);
  const observations: string[] = ["calibrated: nvm"];
  if (nodeVersion.length > 0) {
    observations.push("active node: " + nodeVersion);
  }
  writeState(true, true, false, "", "nvm", "", observations);
}

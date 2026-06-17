# AssemblyScript Integrations — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deliver `manager/nvm`, `tool/just`, and `env/shell` written in AssemblyScript, demonstrating the AS guest ABI as a reference path for TypeScript/JavaScript-background integration authors.

**Architecture:** AssemblyScript compiles TypeScript-like syntax to WASM. JSON is handled via the `assemblyscript-json` library. Each integration is a standalone `assembly/index.ts` compiled with the AssemblyScript compiler (`asc`). The host ABI is imported via `@external` decorators. All three tasks are independent.

**Tech Stack:** AssemblyScript (`assemblyscript` npm package), `assemblyscript-json`, Node.js + npm for toolchain, `asc` compiler, `go test ./integrations/...`

## Global Constraints

- AssemblyScript target: WASM (no WASI)
- All AS integrations export `detect`, `initialize`, `scan`, `calibrate` via `export function`
- Host ABI imported via `@external("orbiter", "<name>") declare function`
- `assemblyscript-json` is the JSON library for all AS integrations
- `integrations/bundle.go` `//go:embed` must be updated for each new integration
- Tests added to `integrations/e2e_test.go`
- Module path: `github.com/Kenttleton/orbiter`
- Run `go test ./integrations/...` after each integration

---

## Common AssemblyScript Pattern

Every AssemblyScript integration uses this file layout:

```
integrations/<brand>/
├── package.json
├── asconfig.json
├── generate.go
├── manifest.toml
└── assembly/
    └── index.ts
```

### Host ABI (identical for all AS integrations)

```typescript
// At top of assembly/index.ts for every integration:

@external("orbiter", "read_input")
declare function read_input(ptr: i32, max: i32): i32;

@external("orbiter", "write_output")
declare function write_output(ptr: i32, len: i32): void;

@external("orbiter", "run_command")
declare function run_command(specPtr: i32, specLen: i32, outPtr: i32, outMax: i32): i32;

const INPUT_BUF_SIZE: i32 = 65536;
const OUTPUT_BUF_SIZE: i32 = 65536;

function readInput(): Uint8Array {
  const buf = new Uint8Array(INPUT_BUF_SIZE);
  const n = read_input(buf.dataStart, buf.byteLength);
  return buf.slice(0, n);
}

function writeStr(s: string): void {
  const encoded = String.UTF8.encode(s, false);
  const buf = Uint8Array.wrap(encoded);
  write_output(buf.dataStart, buf.byteLength);
}

function runCmd(cmd: string, args: string[]): string {
  let spec = '{"cmd":' + JSON.stringify(cmd) + ',"args":[';
  for (let i = 0; i < args.length; i++) {
    if (i > 0) spec += ",";
    spec += JSON.stringify(args[i]);
  }
  spec += "]}";
  const specEncoded = String.UTF8.encode(spec, false);
  const specBuf = Uint8Array.wrap(specEncoded);
  const outBuf = new Uint8Array(OUTPUT_BUF_SIZE);
  const n = run_command(specBuf.dataStart, specBuf.byteLength, outBuf.dataStart, outBuf.byteLength);
  return String.UTF8.decode(outBuf.buffer.slice(0, n)).trimEnd();
}
```

### asconfig.json (identical for all AS integrations)

```json
{
  "targets": {
    "release": {
      "outFile": "build/<brand>.wasm",
      "optimizeLevel": 3,
      "shrinkLevel": 2,
      "noAssert": true
    }
  },
  "options": {
    "exportRuntime": false
  }
}
```

### package.json (per-integration — only brand name changes)

```json
{
  "name": "<brand>-integration",
  "version": "1.0.0",
  "private": true,
  "scripts": {
    "asbuild": "asc assembly/index.ts --target release"
  },
  "devDependencies": {
    "assemblyscript": "^0.27.0",
    "assemblyscript-json": "^1.1.0"
  }
}
```

### generate.go (per-integration)

```go
package <brand>

//go:generate sh -c "npm install && npm run asbuild && cp build/<brand>.wasm ."
```

---

### Task 1: manager/nvm

**Files:**
- Create: `integrations/nvm/package.json`
- Create: `integrations/nvm/asconfig.json`
- Create: `integrations/nvm/generate.go`
- Create: `integrations/nvm/manifest.toml`
- Create: `integrations/nvm/assembly/index.ts`
- Modify: `integrations/bundle.go`
- Modify: `integrations/e2e_test.go`

- [ ] **Step 1: Write failing test**

Add to `integrations/e2e_test.go`:

```go
func TestBundledIntegrations_NVM(t *testing.T) {
	reg := setupBundleRegistry(t)
	i, ok := reg.Get("manager", "nvm")
	if !ok {
		t.Fatal("nvm integration not registered")
	}

	t.Run("detect_nvmrc", func(t *testing.T) {
		report := i.Detect(core.DetectContext{
			Files: map[string]string{".nvmrc": ""},
		})
		if !report.Detected {
			t.Error("expected detected=true for .nvmrc")
		}
		if len(report.Resources) == 0 || report.Resources[0].Role != "manager" {
			t.Error("expected role=manager suggestion")
		}
		if report.Resources[0].Brand != "nvm" {
			t.Errorf("expected brand=nvm, got %q", report.Resources[0].Brand)
		}
	})

	t.Run("detect_node_version", func(t *testing.T) {
		report := i.Detect(core.DetectContext{
			Files: map[string]string{".node-version": ""},
		})
		if !report.Detected {
			t.Error("expected detected=true for .node-version")
		}
	})

	t.Run("detect_miss", func(t *testing.T) {
		report := i.Detect(core.DetectContext{
			Files: map[string]string{"go.mod": ""},
		})
		if report.Detected {
			t.Error("expected detected=false without nvm files")
		}
	})

	t.Run("scan", func(t *testing.T) {
		report := i.Scan(core.ResolvedContext{})
		t.Logf("Scan: %+v", report)
		if report.Manager == "" {
			t.Error("expected non-empty manager field")
		}
	})
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./integrations/... -run TestBundledIntegrations_NVM -v
```

Expected: FAIL — "nvm integration not registered"

- [ ] **Step 3: Create directory structure**

```bash
mkdir -p integrations/nvm/assembly integrations/nvm/build
```

- [ ] **Step 4: Create package.json**

Create `integrations/nvm/package.json`:

```json
{
  "name": "nvm-integration",
  "version": "1.0.0",
  "private": true,
  "scripts": {
    "asbuild": "asc assembly/index.ts --target release"
  },
  "devDependencies": {
    "assemblyscript": "^0.27.0",
    "assemblyscript-json": "^1.1.0"
  }
}
```

- [ ] **Step 5: Create asconfig.json**

Create `integrations/nvm/asconfig.json`:

```json
{
  "targets": {
    "release": {
      "outFile": "build/nvm.wasm",
      "optimizeLevel": 3,
      "shrinkLevel": 2,
      "noAssert": true
    }
  },
  "options": {
    "exportRuntime": false
  }
}
```

- [ ] **Step 6: Write assembly/index.ts**

Create `integrations/nvm/assembly/index.ts`:

```typescript
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
  const nvmDir = runCmd("sh", ["-c", "echo $NVM_DIR"]);
  const nvmPath = nvmDir.length > 0 ? nvmDir : runCmd("which", ["nvm"]);
  if (nvmPath.length == 0) {
    writeState(false, false, false, "", "system", "nvm not found — NVM_DIR not set", []);
    return;
  }
  const nodeVersion = runCmd("node", ["--version"]);
  const observations: string[] = [
    "nvm dir: " + nvmPath,
  ];
  if (nodeVersion.length > 0) {
    observations.push("active node: " + nodeVersion);
  }
  writeState(true, true, true, nvmPath, "nvm", "", observations);
}

export function scan(): void {
  initialize();
}

export function calibrate(): void {
  readInput();
  const nvmDir = runCmd("sh", ["-c", "echo $NVM_DIR"]);
  if (nvmDir.length == 0) {
    writeState(false, false, false, "", "system", "nvm not found", []);
    return;
  }
  const nodeVersion = runCmd("node", ["--version"]);
  writeState(true, true, true, nvmDir, "nvm", "", [
    "calibrated: nvm at " + nvmDir,
    "active node: " + nodeVersion,
  ]);
}
```

- [ ] **Step 7: Create generate.go**

Create `integrations/nvm/generate.go`:

```go
package nvm

//go:generate sh -c "npm install && npm run asbuild && cp build/nvm.wasm ."
```

- [ ] **Step 8: Create manifest.toml**

Create `integrations/nvm/manifest.toml`:

```toml
[integration]
brand = "nvm"
name = "nvm"
description = "Scans and verifies the Node Version Manager"
roles = ["manager"]

[detection]
files = [".nvmrc", ".node-version"]

[dependencies]
  [dependencies.resources]
  runtime = ["node"]

[commands]
allowed = ["node", "nvm", "sh", "which"]
timeout_seconds = 30

[runtime]
pool_size = 4
input_buffer_kb = 8
output_buffer_kb = 8
```

- [ ] **Step 9: Update bundle.go embed**

Add `nvm/nvm.wasm nvm/manifest.toml` to the `//go:embed` directive in `integrations/bundle.go`.

- [ ] **Step 10: Install deps and compile**

```bash
cd integrations/nvm && npm install && go generate .
```

Expected: `nvm.wasm` created in `integrations/nvm/`.

- [ ] **Step 11: Run tests**

```bash
go test ./integrations/... -run TestBundledIntegrations_NVM -v
```

Expected: all subtests pass.

- [ ] **Step 12: Commit**

```bash
git add integrations/nvm/ integrations/bundle.go integrations/e2e_test.go
git commit -m "feat: add manager/nvm AssemblyScript integration"
```

---

### Task 2: tool/just

**Files:**
- Create: `integrations/just/package.json`
- Create: `integrations/just/asconfig.json`
- Create: `integrations/just/generate.go`
- Create: `integrations/just/manifest.toml`
- Create: `integrations/just/assembly/index.ts`
- Modify: `integrations/bundle.go`
- Modify: `integrations/e2e_test.go`

- [ ] **Step 1: Write failing test**

Add to `integrations/e2e_test.go`:

```go
func TestBundledIntegrations_Just(t *testing.T) {
	reg := setupBundleRegistry(t)
	i, ok := reg.Get("tool", "just")
	if !ok {
		t.Fatal("just integration not registered")
	}

	t.Run("detect_justfile", func(t *testing.T) {
		report := i.Detect(core.DetectContext{
			Files: map[string]string{"Justfile": ""},
		})
		if !report.Detected {
			t.Error("expected detected=true for Justfile")
		}
		if len(report.Resources) == 0 || report.Resources[0].Role != "tool" {
			t.Error("expected role=tool suggestion")
		}
	})

	t.Run("detect_lowercase", func(t *testing.T) {
		report := i.Detect(core.DetectContext{
			Files: map[string]string{"justfile": ""},
		})
		if !report.Detected {
			t.Error("expected detected=true for lowercase justfile")
		}
	})

	t.Run("detect_miss", func(t *testing.T) {
		report := i.Detect(core.DetectContext{
			Files: map[string]string{"Makefile": ""},
		})
		if report.Detected {
			t.Error("expected detected=false without Justfile")
		}
	})

	t.Run("scan", func(t *testing.T) {
		report := i.Scan(core.ResolvedContext{})
		t.Logf("Scan: %+v", report)
		if report.Manager == "" {
			t.Error("expected non-empty manager field")
		}
	})
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./integrations/... -run TestBundledIntegrations_Just -v
```

Expected: FAIL — "just integration not registered"

- [ ] **Step 3: Create directory structure**

```bash
mkdir -p integrations/just/assembly integrations/just/build
```

- [ ] **Step 4: Create package.json and asconfig.json**

Create `integrations/just/package.json`:

```json
{
  "name": "just-integration",
  "version": "1.0.0",
  "private": true,
  "scripts": {
    "asbuild": "asc assembly/index.ts --target release"
  },
  "devDependencies": {
    "assemblyscript": "^0.27.0",
    "assemblyscript-json": "^1.1.0"
  }
}
```

Create `integrations/just/asconfig.json`:

```json
{
  "targets": {
    "release": {
      "outFile": "build/just.wasm",
      "optimizeLevel": 3,
      "shrinkLevel": 2,
      "noAssert": true
    }
  },
  "options": {
    "exportRuntime": false
  }
}
```

- [ ] **Step 5: Write assembly/index.ts**

Create `integrations/just/assembly/index.ts`:

```typescript
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
  let hasJustfile = false;
  if (filesVal != null && filesVal.isObj) {
    const files = filesVal as JSON.Obj;
    hasJustfile = files.has("Justfile") || files.has("justfile");
  }
  if (!hasJustfile) {
    writeStr('{"detected":false}');
    return;
  }
  const out = new JSON.Obj();
  out.set("detected", true);
  const resources = new JSON.Arr();
  const resource = new JSON.Obj();
  resource.set("role", "tool");
  resource.set("brand", "just");
  resources.push(resource);
  out.set("resources", resources);
  writeStr(out.stringify());
}

export function initialize(): void {
  readInput();
  const binaryPath = runCmd("which", ["just"]);
  if (binaryPath.length == 0) {
    writeState(false, false, false, "", "system", "just not found in PATH", []);
    return;
  }
  const version = runCmd("just", ["--version"]);
  writeState(true, true, true, binaryPath, "system", "", [version]);
}

export function scan(): void {
  initialize();
}

export function calibrate(): void {
  readInput();
  const version = runCmd("just", ["--version"]);
  if (version.length == 0) {
    writeState(false, false, false, "", "system", "just not found", []);
    return;
  }
  writeState(true, true, true, "", "system", "", ["calibrated: " + version]);
}
```

- [ ] **Step 6: Create generate.go and manifest.toml**

Create `integrations/just/generate.go`:

```go
package just

//go:generate sh -c "npm install && npm run asbuild && cp build/just.wasm ."
```

Create `integrations/just/manifest.toml`:

```toml
[integration]
brand = "just"
name = "just"
description = "Detects and verifies the just command runner"
roles = ["tool"]

[detection]
files = ["Justfile", "justfile"]

[commands]
allowed = ["just", "which"]
timeout_seconds = 30

[runtime]
pool_size = 4
input_buffer_kb = 8
output_buffer_kb = 8
```

- [ ] **Step 7: Update bundle.go and compile**

Add `just/just.wasm just/manifest.toml` to the `//go:embed` directive.

```bash
cd integrations/just && npm install && go generate .
```

- [ ] **Step 8: Run tests**

```bash
go test ./integrations/... -run TestBundledIntegrations_Just -v
```

Expected: all subtests pass.

- [ ] **Step 9: Commit**

```bash
git add integrations/just/ integrations/bundle.go integrations/e2e_test.go
git commit -m "feat: add tool/just AssemblyScript integration"
```

---

### Task 3: env/shell

**Files:**
- Create: `integrations/shell/package.json`
- Create: `integrations/shell/asconfig.json`
- Create: `integrations/shell/generate.go`
- Create: `integrations/shell/manifest.toml`
- Create: `integrations/shell/assembly/index.ts`
- Modify: `integrations/bundle.go`
- Modify: `integrations/e2e_test.go`

The `env/shell` transponder validates that declared environment variable dependencies are present. It always reports as detected (every environment has shell variables). Scan reads from the resolved context to verify which env keys are set and non-empty. This is a read-only transponder.

- [ ] **Step 1: Write failing test**

Add to `integrations/e2e_test.go`:

```go
func TestBundledIntegrations_EnvShell(t *testing.T) {
	reg := setupBundleRegistry(t)
	i, ok := reg.Get("env", "shell")
	if !ok {
		t.Fatal("shell env integration not registered")
	}

	t.Run("detect_always", func(t *testing.T) {
		// env/shell is always detected — every environment has shell variables
		report := i.Detect(core.DetectContext{
			Files: map[string]string{"go.mod": ""},
		})
		if !report.Detected {
			t.Error("expected detected=true for env/shell (always active)")
		}
		if len(report.Resources) == 0 || report.Resources[0].Role != "env" {
			t.Error("expected role=env suggestion")
		}
	})

	t.Run("scan_with_env", func(t *testing.T) {
		// Set up context with some env vars to check
		report := i.Scan(core.ResolvedContext{})
		t.Logf("Scan: %+v", report)
		if !report.Present {
			t.Error("expected present=true for env/shell")
		}
		if report.Manager == "" {
			t.Error("expected non-empty manager field")
		}
	})
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./integrations/... -run TestBundledIntegrations_EnvShell -v
```

Expected: FAIL — "shell env integration not registered"

- [ ] **Step 3: Create directory structure**

```bash
mkdir -p integrations/shell/assembly integrations/shell/build
```

- [ ] **Step 4: Create package.json and asconfig.json**

Create `integrations/shell/package.json`:

```json
{
  "name": "shell-env-integration",
  "version": "1.0.0",
  "private": true,
  "scripts": {
    "asbuild": "asc assembly/index.ts --target release"
  },
  "devDependencies": {
    "assemblyscript": "^0.27.0",
    "assemblyscript-json": "^1.1.0"
  }
}
```

Create `integrations/shell/asconfig.json`:

```json
{
  "targets": {
    "release": {
      "outFile": "build/shell.wasm",
      "optimizeLevel": 3,
      "shrinkLevel": 2,
      "noAssert": true
    }
  },
  "options": {
    "exportRuntime": false
  }
}
```

- [ ] **Step 5: Write assembly/index.ts**

Create `integrations/shell/assembly/index.ts`:

```typescript
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
  const arr = new JSON.Arr();
  for (let i = 0; i < observations.length; i++) {
    arr.push(new JSON.Str(observations[i]));
  }
  obj.set("observations", arr);
  writeStr(obj.stringify());
}

export function scan(): void {
  initialize();
}

// env transponders are read-only — calibrate is the same as scan
export function calibrate(): void {
  initialize();
}
```

- [ ] **Step 6: Create generate.go and manifest.toml**

Create `integrations/shell/generate.go`:

```go
package shell

//go:generate sh -c "npm install && npm run asbuild && cp build/shell.wasm ."
```

Create `integrations/shell/manifest.toml`:

```toml
[integration]
brand = "shell"
name = "Shell Environment"
description = "Validates shell environment variables for dependent resources"
roles = ["env"]

[commands]
allowed = ["sh"]
timeout_seconds = 10

[runtime]
pool_size = 2
input_buffer_kb = 8
output_buffer_kb = 8
```

- [ ] **Step 7: Update bundle.go and compile**

Add `shell/shell.wasm shell/manifest.toml` to the `//go:embed` directive.

```bash
cd integrations/shell && npm install && go generate .
```

- [ ] **Step 8: Run full test suite**

```bash
go test ./integrations/... -run TestBundledIntegrations_EnvShell -v
```

Expected: all subtests pass.

```bash
go test ./integrations/... -v
```

Expected: all existing tests still pass.

- [ ] **Step 9: Commit**

```bash
git add integrations/shell/ integrations/bundle.go integrations/e2e_test.go
git commit -m "feat: add env/shell AssemblyScript integration"
```

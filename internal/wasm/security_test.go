package wasm

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Kenttleton/orbiter/internal/integrations"
)

// These tests try to break out of the wasm sandbox, or to take Orbiter down from inside it. The
// guests are hostile test modules (internal/wasm/testdata/guests/*.wat, assembled by
// `just build-test-guests`); each test runs against both engines, because the guarantee is the
// same on either. What is still open is recorded in docs/security.md, and a test
// below pins the current behaviour so the document and the code cannot drift apart.

func forEachEngine(t *testing.T, fn func(t *testing.T, engine string)) {
	t.Helper()
	for _, engine := range []string{integrations.EngineInterpreter, integrations.EngineCompiler} {
		t.Run(engine, func(t *testing.T) { fn(t, engine) })
	}
}

func readGuest(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/guests/" + name + ".wasm")
	if err != nil {
		t.Fatalf("test guest %s: %v (run `just build-test-guests`)", name, err)
	}
	return b
}

// loadHostile loads a test guest declaring the given commands, approving everything the Captain is
// asked, and returns it with the registry that records a quarantine.
func loadHostile(t *testing.T, engine, guest string, allowed ...string) (*WASMIntegration, *integrations.Registry) {
	t.Helper()
	var m integrations.Manifest
	m.Integration.Brand = "tester"
	m.Runtime.Engine = engine
	for _, c := range allowed {
		m.Commands.Allowed = append(m.Commands.Allowed, integrations.ManifestCommandEntry{Cmd: c})
	}
	reg := integrations.NewRegistry(nil)
	i, err := Load(context.Background(), m, readGuest(t, guest), nil, reg, Prompts{
		Approve:     func(_, _ string) bool { return true },
		AlwaysAllow: func(_, _ string) bool { return false },
	})
	if err != nil {
		t.Fatalf("load %s: %v", guest, err)
	}
	return i, reg
}

// within fails the test if fn does not return in d: a hang is a denial of service.
func within(t *testing.T, d time.Duration, what string, fn func()) {
	t.Helper()
	done := make(chan struct{})
	go func() { fn(); close(done) }()
	select {
	case <-done:
	case <-time.After(d):
		t.Fatalf("%s did not return within %s", what, d)
	}
}

func spec(cmd string, args ...string) []byte {
	b, _ := json.Marshal(map[string]any{"cmd": cmd, "args": args})
	return b
}

// ── The guest cannot take Orbiter down ───────────────────────────────────────

// A trap (out-of-bounds access, unreachable, stack overflow) ends the call with an error. It must
// not crash Orbiter, and it must not leave the integration dead for the next call: the failed
// module is replaced.
func TestSandbox_ATrapEndsTheCallAndTheIntegrationSurvives(t *testing.T) {
	forEachEngine(t, func(t *testing.T, engine string) {
		for _, guest := range []string{"oob", "trap", "recurse"} {
			i, _ := loadHostile(t, engine, guest)
			for call := 1; call <= 3; call++ {
				within(t, 10*time.Second, guest+" call", func() {
					if _, err := i.invoke(context.Background(), "scan", nil); err == nil {
						t.Errorf("%s call %d: expected a trap", guest, call)
					}
				})
			}
		}
	})
}

func TestSandbox_HostFunctionsSurviveGarbagePointers(t *testing.T) {
	forEachEngine(t, func(t *testing.T, engine string) {
		i, _ := loadHostile(t, engine, "host_oob", "true")
		out, err := i.invoke(context.Background(), "scan", nil)
		if err != nil {
			t.Fatalf("pointers outside the guest's memory must be refused, not trap the host: %v", err)
		}
		if string(out) != "OK" {
			t.Errorf("output = %q, want OK", out)
		}
	})
}

// A guest gets exactly the host functions in the orbiter module. It cannot import WASI (no files,
// no console, no clock, no network) or any function Orbiter does not offer.
func TestSandbox_GuestsCannotImportMoreThanTheHostOffers(t *testing.T) {
	forEachEngine(t, func(t *testing.T, engine string) {
		for _, guest := range []string{"wasi", "badimport"} {
			var m integrations.Manifest
			m.Integration.Brand = "tester"
			m.Runtime.Engine = engine
			if _, err := Load(context.Background(), m, readGuest(t, guest), nil, nil, Prompts{}); err == nil {
				t.Errorf("%s: a guest importing what the host does not offer must not load", guest)
			}
		}
	})
}

// A guest that never returns is stopped. Time the host spends waiting for the Captain to answer a
// prompt is not the guest's time.
func TestSandbox_ASpinningGuestIsStopped(t *testing.T) {
	old := GuestTimeout
	GuestTimeout = 300 * time.Millisecond
	t.Cleanup(func() { GuestTimeout = old })
	forEachEngine(t, func(t *testing.T, engine string) {
		i, _ := loadHostile(t, engine, "spin")
		within(t, 10*time.Second, "a spinning guest", func() {
			_, err := i.invoke(context.Background(), "scan", nil)
			if err == nil {
				t.Error("a guest that never returns must end in an error")
			}
		})
	})
}

func TestSandbox_TheCaptainsThinkingTimeIsNotTheGuestsTime(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses true(1)")
	}
	old := GuestTimeout
	GuestTimeout = 300 * time.Millisecond
	t.Cleanup(func() { GuestTimeout = old })
	forEachEngine(t, func(t *testing.T, engine string) {
		var m integrations.Manifest
		m.Integration.Brand = "tester"
		m.Runtime.Engine = engine
		m.Commands.Allowed = []integrations.ManifestCommandEntry{{Cmd: "printf"}}
		i, err := Load(context.Background(), m, readGuest(t, "runspec"), nil, integrations.NewRegistry(nil), Prompts{
			Approve: func(_, _ string) bool { time.Sleep(900 * time.Millisecond); return true },
		})
		if err != nil {
			t.Fatal(err)
		}
		out, err := i.invoke(context.Background(), "scan", spec("printf", "ok"))
		if err != nil {
			t.Fatalf("a call that waited for the Captain must not be killed as if it were computing: %v", err)
		}
		if string(out) != "ok" {
			t.Errorf("output = %q", out)
		}
	})
}

// grewPages runs the grow guest under a manifest declaring memoryBytes (0: omitted) and returns how
// many pages past its first it got.
func grewPages(t *testing.T, engine string, memoryBytes int64) uint32 {
	t.Helper()
	var m integrations.Manifest
	m.Integration.Brand = "tester"
	m.Runtime.Engine = engine
	m.Runtime.MemoryBytes = memoryBytes
	i, err := Load(context.Background(), m, readGuest(t, "grow"), nil, nil, Prompts{})
	if err != nil {
		t.Fatal(err)
	}
	out, err := i.invoke(context.Background(), "scan", nil)
	if err != nil || len(out) != 4 {
		t.Fatalf("grow guest: out=%v err=%v", out, err)
	}
	return binary.LittleEndian.Uint32(out)
}

// A guest cannot take all the memory it likes: the manifest's memory_limit (bytes), else the default.
func TestSandbox_GuestMemoryIsCappedByTheDefault(t *testing.T) {
	forEachEngine(t, func(t *testing.T, engine string) {
		got := grewPages(t, engine, 0)
		if limit := uint32(integrations.DefaultMemoryLimit / 65536); got >= limit {
			t.Errorf("guest grew %d pages past its first, default cap is %d pages in all", got, limit)
		}
	})
}

func TestSandbox_GuestMemoryIsCappedByTheManifest(t *testing.T) {
	forEachEngine(t, func(t *testing.T, engine string) {
		if got := grewPages(t, engine, 8<<20); got >= 128 {
			t.Errorf("guest grew %d pages past its first under an 8 MiB manifest limit (128 pages)", got)
		}
		if got := grewPages(t, engine, 128<<20); got < 128 {
			t.Errorf("a 128 MiB declaration must allow more than 128 pages, got %d", got)
		}
	})
}

func TestSandbox_AMemoryDeclarationPastTheCeilingDoesNotLoad(t *testing.T) {
	var m integrations.Manifest
	m.Integration.Brand = "tester"
	m.Runtime.MemoryBytes = integrations.MaxMemoryLimit + 1
	if _, err := Load(context.Background(), m, readGuest(t, "echo"), nil, nil, Prompts{}); err == nil {
		t.Error("a manifest asking for more memory than the ceiling must not load")
	}
}

// ── run_command: the gate cannot be talked around ────────────────────────────

func needUnix(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("uses unix commands")
	}
}

// The allowlist names commands, not paths. A path whose last element happens to be a declared name
// is some other program, and the integration that asks for it has strayed.
func TestRunCommand_APathCannotImpersonateADeclaredName(t *testing.T) {
	needUnix(t)
	dir := t.TempDir()
	script := filepath.Join(dir, "git")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho PWNED\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	forEachEngine(t, func(t *testing.T, engine string) {
		i, reg := loadHostile(t, engine, "runspec", "git")
		out, _ := i.invoke(context.Background(), "scan", spec(script))
		if strings.Contains(string(out), "PWNED") {
			t.Fatal("a program at an arbitrary path ran because its file name matched a declared command")
		}
		if !reg.IsQuarantined("tester") {
			t.Error("asking for a path outside the declaration must quarantine the integration")
		}
	})
}

func TestRunCommand_AStrayQuarantinesAndStaysBlocked(t *testing.T) {
	needUnix(t)
	forEachEngine(t, func(t *testing.T, engine string) {
		i, reg := loadHostile(t, engine, "runspec", "printf")
		if out, _ := i.invoke(context.Background(), "scan", spec("echo", "x")); len(out) != 0 {
			t.Errorf("an undeclared command produced output %q", out)
		}
		if !reg.IsQuarantined("tester") {
			t.Fatal("the stray must quarantine")
		}
		if out, _ := i.invoke(context.Background(), "scan", spec("printf", "declared")); len(out) != 0 {
			t.Errorf("a quarantined integration ran a declared command: %q", out)
		}
	})
}

func TestRunCommand_ADeclaredCommandRuns(t *testing.T) {
	needUnix(t)
	forEachEngine(t, func(t *testing.T, engine string) {
		i, reg := loadHostile(t, engine, "runspec", "printf")
		out, err := i.invoke(context.Background(), "scan", spec("printf", "hello"))
		if err != nil || string(out) != "hello" {
			t.Errorf("out=%q err=%v", out, err)
		}
		if reg.IsQuarantined("tester") {
			t.Error("an integration that ran what it declared must not be quarantined")
		}
	})
}

func TestRunCommand_MalformedSpecsAreRefusedNotFatal(t *testing.T) {
	needUnix(t)
	huge := make([]string, 4000)
	for k := range huge {
		huge[k] = strings.Repeat("a", 1)
	}
	for name, input := range map[string][]byte{
		"not json":      []byte("{{{{"),
		"empty":         nil,
		"empty command": spec(""),
		"nul in name":   spec("print\x00f"),
		"huge argv":     spec("printf", huge...),
		"wrong types":   []byte(`{"cmd":["printf"],"args":"x"}`),
	} {
		forEachEngine(t, func(t *testing.T, engine string) {
			i, _ := loadHostile(t, engine, "runspec", "printf")
			within(t, 10*time.Second, name, func() {
				_, _ = i.invoke(context.Background(), "scan", input)
			})
		})
	}
}

// A declared command that never ends, or never stops talking, must not hold Orbiter hostage.
func TestRunCommand_ACommandThatNeverStopsIsCutOff(t *testing.T) {
	needUnix(t)
	oldOut, oldTime := MaxCommandOutput, DefaultCommandTimeout
	MaxCommandOutput, DefaultCommandTimeout = 64*1024, 2*time.Second
	t.Cleanup(func() { MaxCommandOutput, DefaultCommandTimeout = oldOut, oldTime })
	forEachEngine(t, func(t *testing.T, engine string) {
		i, _ := loadHostile(t, engine, "runspec", "yes")
		within(t, 10*time.Second, "an endless command", func() {
			_, _ = i.invoke(context.Background(), "scan", spec("yes"))
		})
	})
}

// ── Open work: current behaviour, pinned so docs/security.md stays true ──────

// ── The environment a command sees is what the manifest claims ───────────────

// loadClaiming loads the runspec guest declaring command cmd and the environment it claims.
func loadClaiming(t *testing.T, cmd string, env ...string) *WASMIntegration {
	t.Helper()
	var m integrations.Manifest
	m.Integration.Brand = "tester"
	m.Commands.Allowed = []integrations.ManifestCommandEntry{{Cmd: cmd}}
	m.Commands.Env = env
	i, err := Load(context.Background(), m, readGuest(t, "runspec"), nil, integrations.NewRegistry(nil), Prompts{
		Approve: func(_, _ string) bool { return true },
	})
	if err != nil {
		t.Fatal(err)
	}
	return i
}

// A declared command does not see Orbiter's secrets unless the manifest claims them.
func TestRunCommand_TheCommandSeesOnlyTheEnvironmentTheManifestClaims(t *testing.T) {
	needUnix(t)
	t.Setenv("ORBITER_TEST_SECRET", "hunter2")
	t.Setenv("ORBITER_TEST_CLAIMED", "visible")
	forEachEngine(t, func(t *testing.T, engine string) {
		out, _ := loadClaiming(t, "env", "ORBITER_TEST_CLAIMED").invoke(context.Background(), "scan", spec("env"))
		if strings.Contains(string(out), "hunter2") || strings.Contains(string(out), "ORBITER_TEST_SECRET") {
			t.Error("an unclaimed secret reached the command (output withheld: it is the host environment)")
		}
		if !strings.Contains(string(out), "ORBITER_TEST_CLAIMED=visible") {
			t.Error("a claimed variable did not arrive")
		}
		if strings.Contains(string(out), "PATH=") {
			t.Error("PATH was not claimed, so it must not be passed")
		}
	})
}

// PATH is a variable like any other: passed when the manifest claims it, and the command is still
// found when it does not (Orbiter resolves the command itself).
func TestRunCommand_PathArrivesOnlyWhenClaimed(t *testing.T) {
	needUnix(t)
	out, err := loadClaiming(t, "env", "PATH").invoke(context.Background(), "scan", spec("env"))
	if err != nil || !strings.Contains(string(out), "PATH=") {
		t.Errorf("a claimed PATH must arrive (err=%v)", err)
	}
	out, err = loadClaiming(t, "env").invoke(context.Background(), "scan", spec("env"))
	if err != nil {
		t.Errorf("a command is found without PATH in its environment: %v", err)
	}
	if strings.Contains(string(out), "PATH=") {
		t.Error("PATH arrived without being claimed")
	}
}

func TestRunCommand_APrefixClaimCoversTheFamily(t *testing.T) {
	needUnix(t)
	t.Setenv("ORBITER_FAM_ONE", "1")
	t.Setenv("ORBITER_FAM_TWO", "2")
	t.Setenv("ORBITER_OTHER", "3")
	out, _ := loadClaiming(t, "env", "ORBITER_FAM_*").invoke(context.Background(), "scan", spec("env"))
	for _, want := range []string{"ORBITER_FAM_ONE=1", "ORBITER_FAM_TWO=2"} {
		if !strings.Contains(string(out), want) {
			t.Errorf("missing %s", want)
		}
	}
	if strings.Contains(string(out), "ORBITER_OTHER") {
		t.Error("an unclaimed variable passed (output withheld)")
	}
}

// OPEN (docs/security.md, "Open work" 1: argument-level validation): the gate checks the
// command, not its arguments, so a declared interpreter can be told to run anything. The Captain
// sees the full command line before approving, which is the control.
func TestKnownHole_ADeclaredInterpreterRunsWhatItIsTold(t *testing.T) {
	needUnix(t)
	i, _ := loadHostile(t, integrations.EngineInterpreter, "runspec", "sh")
	out, _ := i.invoke(context.Background(), "scan", spec("sh", "-c", "echo anything"))
	if !strings.Contains(string(out), "anything") {
		t.Error("argument validation now stops this: update docs/security.md and invert this test")
	}
}

// ── Integrations are stateless: spun up for a call, spun down after ──────────

// Every call starts from a fresh instance. What one call's input left in memory is gone in the
// next: Orbiter is the state manager, an integration keeps nothing.
func TestSandbox_NothingSurvivesBetweenCalls(t *testing.T) {
	forEachEngine(t, func(t *testing.T, engine string) {
		i, _ := loadHostile(t, engine, "counter")
		for call := 1; call <= 3; call++ {
			out, err := i.invoke(context.Background(), "scan", nil)
			if err != nil || len(out) != 4 {
				t.Fatalf("call %d: out=%v err=%v", call, out, err)
			}
			if got := binary.LittleEndian.Uint32(out); got != 1 {
				t.Errorf("call %d saw count %d: the guest kept state from an earlier call", call, got)
			}
		}
	})
}

// Instances exist only while a call runs: none is kept alive between calls.
func TestSandbox_NoInstanceIsKeptBetweenCalls(t *testing.T) {
	forEachEngine(t, func(t *testing.T, engine string) {
		i, _ := loadHostile(t, engine, "echo")
		if _, err := i.invoke(context.Background(), "scan", nil); err != nil {
			t.Fatal(err)
		}
		if n := i.live(); n != 0 {
			t.Errorf("%d instances alive after the call returned", n)
		}
	})
}

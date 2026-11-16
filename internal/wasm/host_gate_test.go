package wasm

import (
	"context"
	"strings"
	"testing"

	"github.com/Kenttleton/orbiter/internal/integrations"
)

// The security policy is transparency. An integration declares, in its human-readable manifest,
// the commands it runs, so the Captain can read them before trusting it. Everything else is
// banned: there is no list of forbidden names, only the whitelist the manifest declares. A command
// that was declared is shown to the Captain for approval; a command that was not is a stray, and the
// integration is quarantined.

func gateState(allowed []string, reg *integrations.Registry) *callState {
	return &callState{
		brand:    "tester",
		allowed:  allowed,
		approve:  func(_, _ string) bool { return true },
		registry: reg,
	}
}

// Any executable may be declared, shells included, by name (a path is not a name: see security_test.go): a shell checking its own version is exactly
// what its manifest says it does.
func TestRunGate_AnExecutableTheManifestDeclaresRuns(t *testing.T) {
	reg := integrations.NewRegistry(nil)
	out, code := runGate(context.Background(), gateState([]string{"sh"}, reg), "sh -c echo-declared", "sh", []string{"-c", "echo declared"})

	if code != 0 {
		t.Fatalf("exit code %d, output %q: a declared command must reach the Captain's approval and run", code, out)
	}
	if !strings.Contains(string(out), "declared") {
		t.Errorf("output = %q", out)
	}
	if reg.IsQuarantined("tester") {
		t.Error("an integration that ran what it declared must not be quarantined")
	}
}

func TestRunGate_StrayingFromTheDeclarationQuarantines(t *testing.T) {
	for name, allowed := range map[string][]string{
		"declares other commands":              {"printf"},
		"declares nothing, so nothing may run": nil,
		"declares an empty list":               {},
	} {
		reg := integrations.NewRegistry(nil)
		_, code := runGate(context.Background(), gateState(allowed, reg), "echo whoops", "/bin/echo", []string{"whoops"})
		if code == 0 {
			t.Errorf("%s: an undeclared command must not run", name)
		}
		if !reg.IsQuarantined("tester") {
			t.Errorf("%s: straying from the declaration must quarantine the integration", name)
		}
	}
}

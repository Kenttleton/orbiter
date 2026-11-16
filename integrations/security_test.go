package integrations_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Kenttleton/orbiter/integrations"
	core "github.com/Kenttleton/orbiter/internal/integrations"
	"github.com/Kenttleton/orbiter/internal/wasm"
)

// writeIntegration makes <dir>/<folder>/{manifest.toml,<folder>.wasm} from a prebuilt test guest.
func writeIntegration(t *testing.T, dir, folder, manifest string) {
	t.Helper()
	guest, err := os.ReadFile("../internal/wasm/testdata/guests/echo.wasm")
	if err != nil {
		t.Fatalf("test guest: %v (run `just build-test-guests`)", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, folder), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, folder, "manifest.toml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, folder, folder+".wasm"), guest, 0o644); err != nil {
		t.Fatal(err)
	}
}

// An integration is its folder: the Captain's trust, the quarantine and the audit log all key on the
// brand, so a folder whose manifest claims another integration's brand would inherit that
// integration's trust (and replace it in the registry). It does not load.
func TestLoadInstalled_AFolderCannotClaimAnotherBrand(t *testing.T) {
	dir := t.TempDir()
	writeIntegration(t, dir, "alpha", "[integration]\nbrand = \"alpha\"\nname = \"Real\"\nroles = [\"tool\"]\n")
	writeIntegration(t, dir, "evil", "[integration]\nbrand = \"alpha\"\nname = \"Impostor\"\nroles = [\"tool\"]\n")

	reg := core.NewRegistry(nil)
	approve := func(_, _ string) bool { return true }
	if err := integrations.LoadInstalled(dir, reg, wasm.Prompts{Approve: approve, AlwaysAllow: approve}); err != nil {
		t.Fatal(err)
	}
	got, ok := reg.Get("tool", "alpha")
	if !ok {
		t.Fatal("the real integration must load")
	}
	if name := got.Meta().Integration.Name; name != "Real" {
		t.Errorf("brand alpha is served by %q: an impostor folder displaced the real integration", name)
	}
}

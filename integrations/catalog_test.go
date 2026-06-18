package integrations_test

import (
	"slices"
	"testing"

	"github.com/Kenttleton/orbiter/integrations"
	core "github.com/Kenttleton/orbiter/internal/integrations"
)

func TestCatalogEntries_ReturnsAllBundled(t *testing.T) {
	entries := integrations.CatalogEntries()
	if len(entries) == 0 {
		t.Fatal("expected at least one catalog entry")
	}
	for _, e := range entries {
		if e.Brand == "" {
			t.Errorf("entry missing brand: %+v", e)
		}
		if e.Name == "" {
			t.Errorf("entry missing name: %+v", e)
		}
		if len(e.Roles) == 0 {
			t.Errorf("entry %q missing roles: %+v", e.Brand, e)
		}
	}
}

func TestInstallSelected_RegistersInRegistry(t *testing.T) {
	reg := core.NewRegistry(nil)
	entries := integrations.CatalogEntries()
	if len(entries) == 0 {
		t.Skip("no catalog entries")
	}

	if err := integrations.InstallSelected(entries, reg, nil); err != nil {
		t.Fatalf("InstallSelected: %v", err)
	}

	// All entries should be registered for each of their roles
	for _, e := range entries {
		for _, role := range e.Roles {
			if _, ok := reg.Get(role, e.Brand); !ok {
				t.Errorf("entry %q/%q not registered after InstallSelected", role, e.Brand)
			}
		}
	}
}

func TestDefaultIntegrationsDir_NotEmpty(t *testing.T) {
	dir := integrations.DefaultIntegrationsDir()
	if dir == "" {
		t.Error("DefaultIntegrationsDir should return a non-empty path")
	}
}

func TestLoadInstalled_EmptyDir(t *testing.T) {
	reg := core.NewRegistry(nil)
	approve := func(_, _ string) bool { return true }
	if err := integrations.LoadInstalled(t.TempDir(), reg, approve); err != nil {
		t.Fatalf("LoadInstalled on empty dir: %v", err)
	}
}

func TestCatalog_ContainsJSON(t *testing.T) {
	entries := integrations.CatalogEntries()
	for _, e := range entries {
		if e.Brand == "json" {
			if !slices.Contains(e.Roles, "export") {
				t.Fatalf("json integration found but missing export role; roles: %v", e.Roles)
			}
			return
		}
	}
	t.Fatal("json integration not found in catalog")
}

func TestCatalog_ContainsTmux(t *testing.T) {
	entries := integrations.CatalogEntries()
	for _, e := range entries {
		if e.Brand == "tmux" {
			if !slices.Contains(e.Roles, "multiplexer") {
				t.Fatalf("tmux integration found but missing multiplexer role; roles: %v", e.Roles)
			}
			return
		}
	}
	t.Fatal("tmux integration not found in catalog")
}

func TestLoadInstalled_AfterExtract(t *testing.T) {
	dir := t.TempDir()
	entries := integrations.CatalogEntries()
	if len(entries) == 0 {
		t.Skip("no catalog entries")
	}
	if err := integrations.ExtractSelected(entries, dir); err != nil {
		t.Fatalf("ExtractSelected: %v", err)
	}

	reg := core.NewRegistry(nil)
	approve := func(_, _ string) bool { return true }
	if err := integrations.LoadInstalled(dir, reg, approve); err != nil {
		t.Fatalf("LoadInstalled: %v", err)
	}

	for _, e := range entries {
		for _, role := range e.Roles {
			if _, ok := reg.Get(role, e.Brand); !ok {
				t.Errorf("expected %s/%s registered after LoadInstalled", role, e.Brand)
			}
		}
	}
}

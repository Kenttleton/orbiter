package integrations_test

import (
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

	if err := integrations.InstallSelected(entries, reg); err != nil {
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

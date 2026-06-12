package integrations_test

import (
	"testing"

	"github.com/Kenttleton/orbiter/internal/integrations"
	"github.com/Kenttleton/orbiter/internal/models"
)

func TestEntity_ResourceImplementsInterface(t *testing.T) {
	r := models.Resource{ID: "r1", Role: "runtime", Brand: "go", Config: `{"version":"1.25"}`}
	var e integrations.Entity = r
	if e.GetID() != "r1" {
		t.Errorf("GetID = %q", e.GetID())
	}
	if e.GetRole() != "runtime" {
		t.Errorf("GetRole = %q", e.GetRole())
	}
	if e.GetBrand() != "go" {
		t.Errorf("GetBrand = %q", e.GetBrand())
	}
	if e.GetConfig() != `{"version":"1.25"}` {
		t.Errorf("GetConfig = %q", e.GetConfig())
	}
}

func TestResolvedContext_SelfIsEntity(t *testing.T) {
	r := models.Resource{ID: "r1", Role: "runtime", Brand: "go", Config: "{}"}
	rc := integrations.ResolvedContext{
		Self: r,
	}
	if rc.Self == nil {
		t.Fatal("Self should not be nil")
	}
	if rc.Self.GetID() != "r1" {
		t.Errorf("Self.GetID = %q", rc.Self.GetID())
	}
}

func TestStateReport_NeedsInputAndExports(t *testing.T) {
	report := integrations.StateReport{
		Present:   true,
		Reachable: true,
		NeedsInput: []integrations.InputRequest{
			{Key: "password", Prompt: "Enter password:", Masked: true},
		},
		Exports: map[string]string{"GITHUB_TOKEN": "ghp_abc123"},
	}
	if len(report.NeedsInput) != 1 {
		t.Errorf("NeedsInput len = %d", len(report.NeedsInput))
	}
	if !report.NeedsInput[0].Masked {
		t.Error("expected Masked = true")
	}
	if report.Exports["GITHUB_TOKEN"] != "ghp_abc123" {
		t.Errorf("Exports[GITHUB_TOKEN] = %q", report.Exports["GITHUB_TOKEN"])
	}
}

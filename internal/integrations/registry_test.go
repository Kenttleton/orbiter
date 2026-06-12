package integrations_test

import (
	"testing"

	"github.com/Kenttleton/orbiter/internal/integrations"
	"github.com/stretchr/testify/require"
)

type stubIntegration struct{}

func (s *stubIntegration) Meta() integrations.Manifest { return integrations.Manifest{} }
func (s *stubIntegration) Detect(ctx integrations.DetectContext) integrations.DetectReport {
	return integrations.DetectReport{Detected: true}
}
func (s *stubIntegration) Init(ctx integrations.ResolvedContext) integrations.StateReport {
	return integrations.StateReport{Present: true, Manager: "stub"}
}
func (s *stubIntegration) Scan(ctx integrations.ResolvedContext) integrations.StateReport {
	return integrations.StateReport{Present: true, Manager: "stub"}
}
func (s *stubIntegration) Calibrate(ctx integrations.ResolvedContext) integrations.StateReport {
	return integrations.StateReport{Present: true, Manager: "stub"}
}

func TestRegistryGetNotFound(t *testing.T) {
	r := integrations.NewRegistry(nil)
	_, ok := r.Get("manager", "nvm")
	require.False(t, ok)
}

func TestRegistryRegisterAndGet(t *testing.T) {
	r := integrations.NewRegistry(nil)
	r.Register("manager", "test-brand", &stubIntegration{})

	i, ok := r.Get("manager", "test-brand")
	require.True(t, ok)
	require.NotNil(t, i)
}

func TestRegistryAllForRole(t *testing.T) {
	r := integrations.NewRegistry(nil)
	r.Register("tool", "brand-a", &stubIntegration{})
	r.Register("tool", "brand-b", &stubIntegration{})
	r.Register("runtime", "brand-c", &stubIntegration{})

	tools := r.AllForRole("tool")
	require.Len(t, tools, 2)

	runtimes := r.AllForRole("runtime")
	require.Len(t, runtimes, 1)
}

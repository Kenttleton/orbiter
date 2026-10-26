package starchart_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Kenttleton/orbiter/internal/integrations"
	"github.com/Kenttleton/orbiter/internal/models"
	"github.com/Kenttleton/orbiter/internal/starchart"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPlanFinds_OrderAndContent(t *testing.T) {
	m := integrations.Manifest{
		Integration: integrations.ManifestIntegration{Binaries: []string{"brew"}},
		Find: integrations.ManifestFind{
			Files: []string{"Brewfile"}, Dirs: []string{"/opt/homebrew"}, Envs: []string{"HOMEBREW_PREFIX"},
		},
	}
	got, err := starchart.PlanFinds(m)
	require.NoError(t, err)
	assert.Equal(t, []starchart.FindRequest{
		{Op: "FIND_BINARY", Key: "brew"},
		{Op: "FIND_FILE", Key: "Brewfile"},
		{Op: "FIND_DIR", Key: "/opt/homebrew"},
		{Op: "FIND_ENV", Key: "HOMEBREW_PREFIX"},
	}, got)
}

func TestPlanFinds_RejectsPatternWithSpace(t *testing.T) {
	_, err := starchart.PlanFinds(integrations.Manifest{Find: integrations.ManifestFind{Files: []string{"my file"}}})
	assert.Error(t, err)
}

func TestParseFindResponse(t *testing.T) {
	res, err := starchart.ParseFindResponse("FIND_BINARY", `{"status":"ok","matches":["/usr/bin/git","/opt/homebrew/bin/git"]}`)
	require.NoError(t, err)
	require.Len(t, res.Matches, 2)
	assert.Equal(t, "/usr/bin/git", res.Matches[0].Unix)

	res, err = starchart.ParseFindResponse("FIND_BINARY", `{"status":"ok","matches":[]}`)
	require.NoError(t, err)
	assert.Empty(t, res.Matches)

	res, err = starchart.ParseFindResponse("FIND_FILE", `{"status":"not_found","message":"location does not exist: /x"}`)
	require.NoError(t, err)
	assert.Empty(t, res.Matches)

	_, err = starchart.ParseFindResponse("FIND_ENV", `{"status":"error","message":"boom"}`)
	assert.ErrorContains(t, err, "boom")

	res, err = starchart.ParseFindResponse("FIND_ENV", `{"status":"ok","name":"X","value":"v"}`)
	require.NoError(t, err)
	assert.Equal(t, "v", res.Value)

	res, err = starchart.ParseFindResponse("FIND_DIR", `{"status":"ok","path":"/tmp","exists":true}`)
	require.NoError(t, err)
	assert.True(t, res.Exists)

	_, err = starchart.ParseFindResponse("FIND_BINARY", `not json`)
	assert.Error(t, err)
}

func TestApplyFindResults(t *testing.T) {
	rc := integrations.ResolvedContext{}
	p, _ := integrations.ParsePath("/usr/bin/git")
	starchart.ApplyFindResults(&rc, starchart.FindRequest{Op: "FIND_BINARY", Key: "git"}, starchart.FindResult{Matches: []integrations.Path{p}})
	assert.Equal(t, p.IntoNative(), rc.Binaries["git"])

	// two matches: ambiguous, must NOT be injected (the Captain picks in Task 8)
	q, _ := integrations.ParsePath("/opt/homebrew/bin/git")
	starchart.ApplyFindResults(&rc, starchart.FindRequest{Op: "FIND_BINARY", Key: "node"}, starchart.FindResult{Matches: []integrations.Path{p, q}})
	_, ok := rc.Binaries["node"]
	assert.False(t, ok)

	starchart.ApplyFindResults(&rc, starchart.FindRequest{Op: "FIND_ENV", Key: "JAVA_HOME"}, starchart.FindResult{Value: "/jdk"})
	assert.Equal(t, "/jdk", rc.Envs["JAVA_HOME"])
	starchart.ApplyFindResults(&rc, starchart.FindRequest{Op: "FIND_DIR", Key: "/opt"}, starchart.FindResult{Exists: true})
	assert.True(t, rc.Dirs["/opt"])
}

func TestBuildResolvedContext_InjectsRecordedBinary(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "brew")
	require.NoError(t, os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755))
	cfg, _ := json.Marshal(map[string]any{"binaries": map[string]string{"brew": bin, "gone": "/no/such/gone"}})
	r := models.Resource{ID: "r1", Role: "manager", Brand: "brew", Config: string(cfg)}
	m := integrations.Manifest{Integration: integrations.ManifestIntegration{Binaries: []string{"brew", "gone", "unrecorded"}}}

	rc := starchart.BuildResolvedContext(r, starchart.LeveledBranch{}, m)

	assert.Equal(t, bin, rc.Binaries["brew"])
	// Orbiter does not verify the path: a stale record is injected as-is and the
	// integration's own report says whether the binary is still there.
	assert.Equal(t, "/no/such/gone", filepath.ToSlash(rc.Binaries["gone"]))
	_, ok := rc.Binaries["unrecorded"]
	assert.False(t, ok, "a declared binary with no recorded path is simply absent")
}

// brewLike declares one binary and reports a fixed presence.
type brewLike struct{ present bool }

func (b brewLike) Meta() integrations.Manifest {
	return integrations.Manifest{Integration: integrations.ManifestIntegration{Brand: "brew", Binaries: []string{"brew"}}}
}
func (b brewLike) Detect(integrations.DetectContext) integrations.DetectReport {
	return integrations.DetectReport{}
}
func (b brewLike) Init(integrations.ResolvedContext) integrations.StateReport {
	return integrations.StateReport{Present: b.present, Reachable: b.present}
}
func (b brewLike) Scan(integrations.ResolvedContext) integrations.StateReport {
	return integrations.StateReport{Present: b.present, Reachable: b.present}
}
func (b brewLike) Calibrate(integrations.ResolvedContext) integrations.StateReport {
	return integrations.StateReport{Present: b.present, Reachable: b.present}
}

func neededFor(t *testing.T, present bool, config string) []starchart.ResourceFind {
	t.Helper()
	ctx := t.Context()
	reg := integrations.NewRegistry(nil)
	reg.Register("tool", "brew", brewLike{present: present})
	sc := testDBWithRegistry(t, reg)
	g, err := sc.CreateGalaxy(ctx, "acme")
	require.NoError(t, err)
	p, err := sc.CreatePlanet(ctx, "api", g.ID, "")
	require.NoError(t, err)
	_, err = sc.CreateResource(ctx, "brew", "tool", "brew", "[]", config)
	require.NoError(t, err)
	_, err = sc.Attach(ctx, "brew", "api")
	require.NoError(t, err)
	scan, err := sc.ScanBranch(ctx, p.ID)
	require.NoError(t, err)
	return sc.NeededFinds(scan)
}

func TestNeededFinds_NoRecordedPath(t *testing.T) {
	got := neededFor(t, false, `{}`)
	require.Len(t, got, 1)
	assert.Equal(t, starchart.FindRequest{Op: "FIND_BINARY", Key: "brew"}, got[0].Request)
	assert.NotEmpty(t, got[0].ResourceID)
}

func TestNeededFinds_RecordedButIntegrationReportsNotPresent(t *testing.T) {
	got := neededFor(t, false, `{"binaries":{"brew":"/old/brew"}}`)
	require.Len(t, got, 1)
	assert.Equal(t, "brew", got[0].Request.Key)
}

func TestNeededFinds_RecordedAndPresent(t *testing.T) {
	assert.Empty(t, neededFor(t, true, `{"binaries":{"brew":"/opt/homebrew/bin/brew"}}`))
}

func TestPlanFinds_RejectsUnsafeBinaryNames(t *testing.T) {
	for _, name := range []string{"git\nSET PATH=/tmp/evil", "../../tmp/x", "a b", "a/b", "", "x\r"} {
		_, err := starchart.PlanFinds(integrations.Manifest{Integration: integrations.ManifestIntegration{Binaries: []string{name}}})
		assert.Error(t, err, "%q must be rejected", name)
	}
	_, err := starchart.PlanFinds(integrations.Manifest{Integration: integrations.ManifestIntegration{Binaries: []string{"ssh-agent", "python3", "g++", "node.exe"}}})
	assert.NoError(t, err)
}

type evilNameIntegration struct{ brewLike }

func (evilNameIntegration) Meta() integrations.Manifest {
	return integrations.Manifest{Integration: integrations.ManifestIntegration{Brand: "brew", Binaries: []string{"git\nSET PATH=/tmp/evil"}}}
}

func TestNeededFinds_SkipsUnsafeBinaryNames(t *testing.T) {
	ctx := t.Context()
	reg := integrations.NewRegistry(nil)
	reg.Register("tool", "brew", evilNameIntegration{})
	sc := testDBWithRegistry(t, reg)
	g, err := sc.CreateGalaxy(ctx, "acme")
	require.NoError(t, err)
	p, err := sc.CreatePlanet(ctx, "api", g.ID, "")
	require.NoError(t, err)
	_, err = sc.CreateResource(ctx, "brew", "tool", "brew", "[]", `{}`)
	require.NoError(t, err)
	_, err = sc.Attach(ctx, "brew", "api")
	require.NoError(t, err)
	scan, err := sc.ScanBranch(ctx, p.ID)
	require.NoError(t, err)
	assert.Empty(t, sc.NeededFinds(scan), "a manifest must not smuggle directives through binary names")
}

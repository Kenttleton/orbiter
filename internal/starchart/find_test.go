package starchart_test

import (
	"testing"

	"github.com/Kenttleton/orbiter/internal/integrations"
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

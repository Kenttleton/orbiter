package commands_test

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kenttleton/orbiter/internal/commands"
	"github.com/Kenttleton/orbiter/internal/output"
	"github.com/Kenttleton/orbiter/internal/starchart"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// surveyFixture: vessel -> acme -> platform (system) -> api (planet, with a go
// resource whose config and a beacon observation exist), plus web (planet in acme).
func surveyFixture(t *testing.T, format string) (*commands.Executor, *bytes.Buffer) {
	t.Helper()
	sc, err := starchart.Open(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { sc.Close() })
	ctx := context.Background()
	g, err := sc.CreateGalaxy(ctx, "acme")
	require.NoError(t, err)
	sys, err := sc.CreateSolarSystem(ctx, "platform", g.ID)
	require.NoError(t, err)
	_, err = sc.CreatePlanet(ctx, "api", g.ID, sys.ID)
	require.NoError(t, err)
	_, err = sc.CreatePlanet(ctx, "web", g.ID, "")
	require.NoError(t, err)
	_, err = sc.CreateResource(ctx, "go", "runtime", "go", "[]", `{"version":"1.22","secret_marker":"cfg-xyz"}`)
	require.NoError(t, err)
	_, err = sc.Attach(ctx, "go", "api")
	require.NoError(t, err)
	buf := &bytes.Buffer{}
	return commands.NewExecutor(sc, output.NewRendererTo(format, false, buf)), buf
}

func TestSurvey_DefaultListsDirectChildrenOnly(t *testing.T) {
	exec, buf := surveyFixture(t, output.FormatStyled)
	require.NoError(t, exec.Survey(context.Background(), "vessel", commands.SurveyOptions{}))
	out := buf.String()
	assert.Contains(t, out, "acme", "the vessel's galaxies are listed")
	assert.NotContains(t, out, "platform", "grandchildren need -a")
	assert.NotContains(t, out, "api")
}

func TestSurvey_AllDescendsTheWholeSubtree(t *testing.T) {
	exec, buf := surveyFixture(t, output.FormatStyled)
	require.NoError(t, exec.Survey(context.Background(), "vessel", commands.SurveyOptions{All: true}))
	out := buf.String()
	for _, name := range []string{"acme", "platform", "api", "web"} {
		assert.Contains(t, out, name)
	}
}

func TestSurvey_VerboseAddsConfigAndObservations(t *testing.T) {
	exec, buf := surveyFixture(t, output.FormatStyled)
	require.NoError(t, exec.Survey(context.Background(), "api", commands.SurveyOptions{}))
	assert.NotContains(t, buf.String(), "cfg-xyz", "config is detail, shown only with -v")

	exec, buf = surveyFixture(t, output.FormatStyled)
	require.NoError(t, exec.Survey(context.Background(), "api", commands.SurveyOptions{Verbose: true}))
	assert.Contains(t, buf.String(), "cfg-xyz")
}

// surveyResult decodes the single result event survey emits in JSON mode.
func surveyResult(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	require.Len(t, lines, 1, "JSON mode emits exactly one event, got %q", buf.String())
	var e output.Event
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &e))
	require.Equal(t, "result", e.Type)
	var res struct {
		Success bool           `json:"success"`
		Data    map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(e.Data, &res))
	require.True(t, res.Success)
	return res.Data
}

func childNames(node map[string]any) []string {
	var out []string
	kids, _ := node["children"].([]any)
	for _, k := range kids {
		out = append(out, k.(map[string]any)["name"].(string))
	}
	return out
}

func TestSurvey_JSONIsOneResultWithTheTree(t *testing.T) {
	exec, buf := surveyFixture(t, output.FormatJSON)
	require.NoError(t, exec.Survey(context.Background(), "vessel", commands.SurveyOptions{}))
	node := surveyResult(t, buf)
	assert.Equal(t, "vessel", node["type"])
	assert.Equal(t, []string{"acme"}, childNames(node))
	acme := node["children"].([]any)[0].(map[string]any)
	assert.Empty(t, childNames(acme), "default JSON is one level deep, like the styled view")
}

func TestSurvey_JSONWithAllIsTheSubtreeAndAlwaysFullDetail(t *testing.T) {
	exec, buf := surveyFixture(t, output.FormatJSON)
	require.NoError(t, exec.Survey(context.Background(), "vessel", commands.SurveyOptions{All: true}))
	node := surveyResult(t, buf)
	acme := node["children"].([]any)[0].(map[string]any)
	assert.ElementsMatch(t, []string{"platform", "web"}, childNames(acme))
	assert.Contains(t, buf.String(), "cfg-xyz", "JSON always carries full detail, with or without -v")
}

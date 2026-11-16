package starchart_test

import (
	"testing"

	"github.com/Kenttleton/orbiter/internal/models"
	"github.com/Kenttleton/orbiter/internal/starchart"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func nodeNames(nodes []starchart.Node) []string {
	var out []string
	for _, n := range nodes {
		out = append(out, n.Type+":"+n.Name)
	}
	return out
}

func TestChildrenOf_WalksTheHierarchy(t *testing.T) {
	ctx := t.Context()
	sc := testDB(t)
	vessel, err := sc.GetVessel(ctx)
	require.NoError(t, err)
	g, err := sc.CreateGalaxy(ctx, "acme")
	require.NoError(t, err)
	sys, err := sc.CreateSolarSystem(ctx, "platform", g.ID)
	require.NoError(t, err)
	_, err = sc.CreatePlanet(ctx, "api", g.ID, sys.ID)
	require.NoError(t, err)
	_, err = sc.CreatePlanet(ctx, "web", g.ID, "")
	require.NoError(t, err)
	api, err := sc.Resolve(ctx, "api")
	require.NoError(t, err)

	kids, err := sc.ChildrenOf(ctx, vessel.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{"gx:acme"}, nodeNames(kids), "the vessel's children are its galaxies")

	kids, err = sc.ChildrenOf(ctx, g.ID)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"sy:platform", "pl:web"}, nodeNames(kids),
		"a galaxy holds its systems and the planets not inside a system")

	kids, err = sc.ChildrenOf(ctx, sys.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{"pl:api"}, nodeNames(kids))

	kids, err = sc.ChildrenOf(ctx, api.ID)
	require.NoError(t, err)
	assert.Empty(t, kids, "a planet has no hierarchy children")
}

func TestAttachedTo_ListsDirectAttachments(t *testing.T) {
	ctx := t.Context()
	sc := testDB(t)
	g, err := sc.CreateGalaxy(ctx, "acme")
	require.NoError(t, err)
	_, err = sc.CreatePlanet(ctx, "api", g.ID, "")
	require.NoError(t, err)
	_, err = sc.CreateResource(ctx, "go", "runtime", "go", "[]", `{}`)
	require.NoError(t, err)
	_, err = sc.CreateCallsign(ctx, "work")
	require.NoError(t, err)
	_, err = sc.CreateTransponder(ctx, "gh", "env", "github", `{}`)
	require.NoError(t, err)
	for _, from := range []string{"go", "work", "gh"} {
		_, err = sc.Attach(ctx, from, "api")
		require.NoError(t, err)
	}
	api, err := sc.Resolve(ctx, "api")
	require.NoError(t, err)

	got, err := sc.AttachedTo(ctx, api.ID)
	require.NoError(t, err)
	require.Len(t, got.Resources, 1)
	assert.Equal(t, "go", got.Resources[0].Name)
	assert.Equal(t, "runtime", got.Resources[0].Role)
	require.Len(t, got.Callsigns, 1)
	assert.Equal(t, "work", got.Callsigns[0].Name)
	require.Len(t, got.Transponders, 1)
	assert.Equal(t, "gh", got.Transponders[0].Name)
	_ = models.EntityTypeGalaxy
}

// How many things in the tree use an integration: resources and transponders of that brand.
func TestCountUsing_CountsResourcesAndTranspondersOfABrand(t *testing.T) {
	ctx := t.Context()
	sc := testDB(t)
	_, err := sc.CreateResource(ctx, "a", "tool", "git", "[]", "{}")
	require.NoError(t, err)
	_, err = sc.CreateResource(ctx, "b", "tool", "git", "[]", "{}")
	require.NoError(t, err)
	_, err = sc.CreateResource(ctx, "c", "runtime", "golang", "[]", "{}")
	require.NoError(t, err)
	_, err = sc.CreateTransponder(ctx, "t", "file", "git", "{}")
	require.NoError(t, err)

	n, err := sc.CountUsing(ctx, "git")
	require.NoError(t, err)
	assert.Equal(t, 3, n)
	n, err = sc.CountUsing(ctx, "nothing")
	require.NoError(t, err)
	assert.Equal(t, 0, n)
}

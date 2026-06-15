package starchart_test

import (
	"context"
	"testing"

	"github.com/Kenttleton/orbiter/internal/models"
	"github.com/Kenttleton/orbiter/internal/starchart"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPlanRetro_UnsharedResource(t *testing.T) {
	sc := testDB(t)
	ctx := context.Background()

	g, err := sc.CreateGalaxy(ctx, "acme")
	require.NoError(t, err)
	p, err := sc.CreatePlanet(ctx, "payment-api", g.ID, "")
	require.NoError(t, err)
	r, err := sc.CreateResource(ctx, "node", "runtime", "node", "[]", "{}")
	require.NoError(t, err)
	_, err = sc.Attach(ctx, "node", "payment-api")
	require.NoError(t, err)

	plan, err := sc.PlanRetro(ctx, p.ID)
	require.NoError(t, err)

	// Expect planet + resource in the plan.
	require.Len(t, plan.Nodes, 2)
	actions := map[string]string{}
	for _, n := range plan.Nodes {
		actions[n.EntityID] = n.Action
	}
	assert.Equal(t, "retire", actions[p.ID])
	assert.Equal(t, "retire", actions[r.ID])
}

func TestPlanRetro_SharedResource(t *testing.T) {
	sc := testDB(t)
	ctx := context.Background()

	g, err := sc.CreateGalaxy(ctx, "acme")
	require.NoError(t, err)
	p1, err := sc.CreatePlanet(ctx, "payment-api", g.ID, "")
	require.NoError(t, err)
	p2, err := sc.CreatePlanet(ctx, "auth-service", g.ID, "")
	require.NoError(t, err)
	r, err := sc.CreateResource(ctx, "node", "runtime", "node", "[]", "{}")
	require.NoError(t, err)
	_, err = sc.Attach(ctx, "node", "payment-api")
	require.NoError(t, err)
	_, err = sc.Attach(ctx, "node", "auth-service")
	require.NoError(t, err)

	plan, err := sc.PlanRetro(ctx, p1.ID)
	require.NoError(t, err)

	actions := map[string]string{}
	for _, n := range plan.Nodes {
		actions[n.EntityID] = n.Action
	}
	assert.Equal(t, "retire", actions[p1.ID])
	assert.Equal(t, "detach", actions[r.ID])
	// p2 is not in the subtree of p1 — it must not appear.
	_, found := actions[p2.ID]
	assert.False(t, found)
}

func TestExecuteRetro_Galaxy(t *testing.T) {
	sc := testDB(t)
	ctx := context.Background()

	g, err := sc.CreateGalaxy(ctx, "acme")
	require.NoError(t, err)
	p, err := sc.CreatePlanet(ctx, "payment-api", g.ID, "")
	require.NoError(t, err)
	r, err := sc.CreateResource(ctx, "node", "runtime", "node", "[]", "{}")
	require.NoError(t, err)
	_, err = sc.Attach(ctx, "node", "payment-api")
	require.NoError(t, err)

	plan, err := sc.PlanRetro(ctx, g.ID)
	require.NoError(t, err)

	// Plan must include galaxy, planet, and resource.
	require.Len(t, plan.Nodes, 3)

	require.NoError(t, sc.ExecuteRetro(ctx, plan))

	var galaxy models.Galaxy
	err = sc.Get(ctx, "galaxies", g.ID, &galaxy)
	assert.ErrorIs(t, err, starchart.ErrNotFound, "galaxy should have been retired")

	var planet models.Planet
	err = sc.Get(ctx, "planets", p.ID, &planet)
	assert.ErrorIs(t, err, starchart.ErrNotFound, "planet should have been retired")

	var resource models.Resource
	err = sc.Get(ctx, "resources", r.ID, &resource)
	assert.ErrorIs(t, err, starchart.ErrNotFound, "resource should have been retired")
}

func TestExecuteRetro(t *testing.T) {
	sc := testDB(t)
	ctx := context.Background()

	g, err := sc.CreateGalaxy(ctx, "acme")
	require.NoError(t, err)
	p, err := sc.CreatePlanet(ctx, "payment-api", g.ID, "")
	require.NoError(t, err)
	r, err := sc.CreateResource(ctx, "node", "runtime", "node", "[]", "{}")
	require.NoError(t, err)
	_, err = sc.Attach(ctx, "node", "payment-api")
	require.NoError(t, err)

	plan, err := sc.PlanRetro(ctx, p.ID)
	require.NoError(t, err)
	require.NoError(t, sc.ExecuteRetro(ctx, plan))

	var planet models.Planet
	err = sc.Get(ctx, "planets", p.ID, &planet)
	assert.ErrorIs(t, err, starchart.ErrNotFound, "planet should have been retired")

	// Resource should also be gone (unshared, retired along with planet).
	var resource models.Resource
	err = sc.Get(ctx, "resources", r.ID, &resource)
	assert.ErrorIs(t, err, starchart.ErrNotFound, "resource should have been retired")
}

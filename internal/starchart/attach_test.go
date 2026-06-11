package starchart_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAttach(t *testing.T) {
	ctx := context.Background()
	sc := testDB(t)

	g, _ := sc.CreateGalaxy(ctx, "stride-build")
	cs, _ := sc.CreateCallsign(ctx, "work-dev")

	att, err := sc.Attach(ctx, "work-dev", "stride-build")
	require.NoError(t, err)
	require.Equal(t, cs.ID, att.FromID)
	require.Equal(t, g.ID, att.ToID)
}

func TestAttachDuplicateErrors(t *testing.T) {
	ctx := context.Background()
	sc := testDB(t)

	sc.CreateGalaxy(ctx, "stride-build")
	sc.CreateCallsign(ctx, "work-dev")

	_, err := sc.Attach(ctx, "work-dev", "stride-build")
	require.NoError(t, err)
	_, err = sc.Attach(ctx, "work-dev", "stride-build")
	require.Error(t, err)
}

func TestAttachOneCallsignPerNode(t *testing.T) {
	ctx := context.Background()
	sc := testDB(t)

	sc.CreateGalaxy(ctx, "stride-build")
	sc.CreateCallsign(ctx, "dev-a")
	sc.CreateCallsign(ctx, "dev-b")

	_, err := sc.Attach(ctx, "dev-a", "stride-build")
	require.NoError(t, err)

	_, err = sc.Attach(ctx, "dev-b", "stride-build")
	require.Error(t, err, "should reject second callsign on same node")
}

func TestAttachResourceToVessel(t *testing.T) {
	ctx := context.Background()
	sc := testDB(t)

	r, _ := sc.CreateResource(ctx, "nvm-mgr", "manager", "nvm", `["node"]`, `{}`)
	vessel, err := sc.GetVessel(ctx)
	require.NoError(t, err)

	att, err := sc.Attach(ctx, "nvm-mgr", "vessel")
	require.NoError(t, err)
	require.Equal(t, r.ID, att.FromID)
	require.Equal(t, vessel.ID, att.ToID)
}

package starchart_test

import (
	"context"
	"fmt"
	"testing"
)

func TestDebugLeveled2(t *testing.T) {
	sc := testDB(t)
	ctx := context.Background()

	g, err := sc.CreateGalaxy(ctx, "acme")
	fmt.Printf("galaxy: %v err=%v\n", g.ID, err)
	p, err := sc.CreatePlanet(ctx, "payment-api", g.ID, "")
	fmt.Printf("planet: %v err=%v\n", p.ID, err)

	r, err := sc.CreateResource(ctx, "node", "runtime", "node", "[]", "")
	fmt.Printf("resource: %v err=%v\n", r.ID, err)
	att, err := sc.Attach(ctx, "node", "payment-api")
	fmt.Printf("attachment: from=%v to=%v err=%v\n", att.FromID, att.ToID, err)

	lb, err := sc.LeveledBranchCrawl(ctx, p.ID)
	fmt.Printf("levels: %d, err=%v\n", len(lb.Levels), err)
}

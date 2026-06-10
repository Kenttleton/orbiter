package resolver

import (
	"context"

	"github.com/Kenttleton/orbiter/internal/models"
	"github.com/Kenttleton/orbiter/internal/starchart"
)

// Resolver resolves a human-readable name or ID to a fully populated Alias.
// It is dependency-injected into all commands — commands never query the
// Star Chart for aliases directly.
type Resolver interface {
	Resolve(ctx context.Context, input string) (models.Alias, error)
}

// starChartResolver wraps StarChart.Resolve behind the Resolver interface.
type starChartResolver struct {
	sc *starchart.StarChart
}

// New returns a Resolver backed by the given StarChart.
func New(sc *starchart.StarChart) Resolver {
	return &starChartResolver{sc: sc}
}

func (r *starChartResolver) Resolve(ctx context.Context, input string) (models.Alias, error) {
	return r.sc.Resolve(ctx, input)
}

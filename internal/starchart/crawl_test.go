package starchart_test

import (
	"testing"

	"github.com/Kenttleton/orbiter/internal/integrations"
	"github.com/Kenttleton/orbiter/internal/models"
	"github.com/Kenttleton/orbiter/internal/starchart"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildResolvedContext_TranspondersFILO(t *testing.T) {
	// Planet-level transponder and galaxy-level transponder share role/brand.
	// Planet (index 0 in lb.Levels) must win via FILO semantics.
	planet := models.Transponder{ID: "planet-tp", Role: "file", Brand: "github", Config: `{"location":"/planet"}`}
	galaxy := models.Transponder{ID: "galaxy-tp", Role: "file", Brand: "github", Config: `{"location":"/galaxy"}`}

	lb := starchart.LeveledBranch{
		Levels: []starchart.BranchLevel{
			{EntityID: "planet-id", Transponders: []models.Transponder{planet}},
			{EntityID: "galaxy-id", Transponders: []models.Transponder{galaxy}},
		},
	}
	manifest := integrations.Manifest{
		Dependencies: integrations.ManifestDependencies{
			Transponders: map[string][]string{"file": {"github"}},
		},
	}
	self := models.Resource{ID: "res-id", Role: "remote", Brand: "github"}

	rc := starchart.BuildResolvedContext(self, lb, manifest)

	require.Len(t, rc.Transponders["file"], 1, "only one file/github entry — planet supersedes galaxy via FILO")
	assert.Equal(t, planet.ID, rc.Transponders["file"][0].Transponder.ID)
}

func TestBuildResolvedContext_ResourcesFILO(t *testing.T) {
	// Planet-level resource and galaxy-level resource share role/brand.
	// Planet wins.
	planet := models.Resource{ID: "planet-rs", Role: "runtime", Brand: "node", Config: "{}"}
	galaxy := models.Resource{ID: "galaxy-rs", Role: "runtime", Brand: "node", Config: "{}"}

	lb := starchart.LeveledBranch{
		Levels: []starchart.BranchLevel{
			{EntityID: "planet-id", Resources: []models.Resource{planet}},
			{EntityID: "galaxy-id", Resources: []models.Resource{galaxy}},
		},
	}
	manifest := integrations.Manifest{
		Dependencies: integrations.ManifestDependencies{
			Resources: map[string][]string{"runtime": {"node"}},
		},
	}
	self := models.Resource{ID: "res-id", Role: "tool", Brand: "npm"}

	rc := starchart.BuildResolvedContext(self, lb, manifest)

	require.Len(t, rc.Resources["runtime"], 1, "only one runtime/node entry — planet supersedes galaxy via FILO")
	assert.Equal(t, planet.ID, rc.Resources["runtime"][0].Resource.ID)
}

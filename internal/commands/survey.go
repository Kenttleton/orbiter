package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Kenttleton/orbiter/internal/models"
	"github.com/Kenttleton/orbiter/internal/output"
)

// SurveyOptions are survey's own flags. Flag conventions (see docs/cli-conventions.md):
// All widens scope to every descendant (-a). Verbose is the root -v flag, data
// volume: the bulky detail a table would otherwise hide. JSON output ignores
// Verbose (it always carries full detail) and honors All.
type SurveyOptions struct {
	All     bool
	Verbose bool
}

// SurveyNode is one entity in the survey tree. It is the single structure behind
// both the styled tables and the JSON result, so the two cannot drift apart.
// Resources, Transponders and Callsigns are what is attached directly to the
// node; Children are its hierarchy children (one level deep, or the whole
// subtree with All).
type SurveyNode struct {
	Type         string          `json:"type"`
	ID           string          `json:"id"`
	Name         string          `json:"name"`
	Status       string          `json:"status,omitempty"`
	Role         string          `json:"role,omitempty"`
	Brand        string          `json:"brand,omitempty"`
	Manages      json.RawMessage `json:"manages,omitempty"`
	Config       json.RawMessage `json:"config,omitempty"`
	Observations []string        `json:"observations,omitempty"`
	VerifiedAt   *time.Time      `json:"verified_at,omitempty"`
	Resources    []SurveyNode    `json:"resources,omitempty"`
	Transponders []SurveyNode    `json:"transponders,omitempty"`
	Callsigns    []SurveyNode    `json:"callsigns,omitempty"`
	Children     []SurveyNode    `json:"children,omitempty"`
}

var entityTypeNames = map[string]string{
	models.EntityTypeVessel:      "vessel",
	models.EntityTypeGalaxy:      "galaxy",
	models.EntityTypeSolarSystem: "system",
	models.EntityTypePlanet:      "planet",
	models.EntityTypeCallsign:    "callsign",
	models.EntityTypeTransponder: "transponder",
	models.EntityTypeResource:    "resource",
}

func rawJSON(s string) json.RawMessage {
	if s != "" && json.Valid([]byte(s)) {
		return json.RawMessage(s)
	}
	return nil
}

// withBeacon fills the node's status, observations and verification time from its beacon.
func (e *Executor) withBeacon(ctx context.Context, n SurveyNode) SurveyNode {
	b, err := e.sc.GetBeacon(ctx, n.ID)
	if err != nil {
		return n // no beacon yet is fine
	}
	n.Status = b.Status
	var obs []string
	if json.Unmarshal([]byte(b.Observations), &obs) == nil {
		n.Observations = obs
	}
	if !b.VerifiedAt.IsZero() {
		t := b.VerifiedAt
		n.VerifiedAt = &t
	}
	return n
}

// surveyNode builds the survey tree rooted at id. deep follows hierarchy
// children all the way down; otherwise children are listed one level deep with
// no attachments of their own.
func (e *Executor) surveyNode(ctx context.Context, id, name string, deep bool) (SurveyNode, error) {
	parsed, err := models.ParseID(id)
	if err != nil {
		return SurveyNode{}, err
	}
	n := e.withBeacon(ctx, SurveyNode{Type: entityTypeNames[parsed.EntityType], ID: id, Name: name})

	switch parsed.EntityType {
	case models.EntityTypeResource:
		var r models.Resource
		if err := e.sc.Get(ctx, "resources", id, &r); err == nil {
			n.Role, n.Brand, n.Manages, n.Config = r.Role, r.Brand, rawJSON(r.Manages), rawJSON(r.Config)
		}
	case models.EntityTypeTransponder:
		var tp models.Transponder
		if err := e.sc.Get(ctx, "transponders", id, &tp); err == nil {
			n.Role, n.Brand, n.Config = tp.Role, tp.Brand, rawJSON(tp.Config)
		}
	}

	att, err := e.sc.AttachedTo(ctx, id)
	if err != nil {
		return SurveyNode{}, fmt.Errorf("attachments of %s: %w", name, err)
	}
	for _, r := range att.Resources {
		n.Resources = append(n.Resources, e.withBeacon(ctx, SurveyNode{
			Type: "resource", ID: r.ID, Name: r.Name, Role: r.Role, Brand: r.Brand,
			Manages: rawJSON(r.Manages), Config: rawJSON(r.Config),
		}))
	}
	for _, tp := range att.Transponders {
		n.Transponders = append(n.Transponders, e.withBeacon(ctx, SurveyNode{
			Type: "transponder", ID: tp.ID, Name: tp.Name, Role: tp.Role, Brand: tp.Brand, Config: rawJSON(tp.Config),
		}))
	}
	for _, c := range att.Callsigns {
		n.Callsigns = append(n.Callsigns, e.withBeacon(ctx, SurveyNode{Type: "callsign", ID: c.ID, Name: c.Name}))
	}

	kids, err := e.sc.ChildrenOf(ctx, id)
	if err != nil {
		return SurveyNode{}, fmt.Errorf("children of %s: %w", name, err)
	}
	for _, k := range kids {
		if deep {
			child, err := e.surveyNode(ctx, k.ID, k.Name, true)
			if err != nil {
				return SurveyNode{}, err
			}
			n.Children = append(n.Children, child)
			continue
		}
		n.Children = append(n.Children, e.withBeacon(ctx, SurveyNode{Type: entityTypeNames[k.Type], ID: k.ID, Name: k.Name}))
	}
	return n, nil
}

// renderSurvey prints what the target's own summary does not: its hierarchy
// children, the -v detail, and with -a every descendant. top marks the surveyed
// target, whose summary tables are printed by surveySummary.
func (e *Executor) renderSurvey(n SurveyNode, opts SurveyOptions, top bool) {
	if !top {
		e.renderer.Info(fmt.Sprintf("%s %s", n.Type, n.Name))
		e.renderer.Table([]string{"field", "value"}, [][]string{
			{"type", n.Type}, {"name", n.Name}, {"id", n.ID}, {"status", n.Status},
		})
		var rows [][]string
		for _, r := range n.Resources {
			rows = append(rows, []string{"resource", r.Name, r.Role + "/" + r.Brand, r.Status})
		}
		for _, tp := range n.Transponders {
			rows = append(rows, []string{"transponder", tp.Name, tp.Role + "/" + tp.Brand, tp.Status})
		}
		for _, c := range n.Callsigns {
			rows = append(rows, []string{"callsign", c.Name, c.ID, c.Status})
		}
		if len(rows) > 0 {
			e.renderer.Table([]string{"attached", "name", "kind", "status"}, rows)
		}
	}
	if len(n.Children) > 0 {
		var rows [][]string
		for _, c := range n.Children {
			rows = append(rows, []string{c.Type, c.Name, c.ID, c.Status})
		}
		e.renderer.Table([]string{"children", "name", "id", "status"}, rows)
	}
	if opts.Verbose {
		e.renderDetail(n)
	}
	if opts.All {
		for _, c := range n.Children {
			e.renderSurvey(c, opts, false)
		}
	}
}

// renderDetail is the -v table: the bulky fields a summary table would hide.
func (e *Executor) renderDetail(n SurveyNode) {
	row := func(x SurveyNode) []string {
		verified := ""
		if x.VerifiedAt != nil {
			verified = x.VerifiedAt.Format(time.RFC3339)
		}
		return []string{x.Name, x.Type, x.Status, verified, string(x.Config), strings.Join(x.Observations, "; ")}
	}
	rows := [][]string{row(n)}
	for _, group := range [][]SurveyNode{n.Resources, n.Transponders, n.Callsigns} {
		for _, x := range group {
			rows = append(rows, row(x))
		}
	}
	e.renderer.Table([]string{"name", "type", "status", "verified", "config", "observations"}, rows)
}

// finishSurvey renders the tree-driven additions and emits the JSON result.
func (e *Executor) finishSurvey(ctx context.Context, id, name string, opts SurveyOptions) error {
	node, err := e.surveyNode(ctx, id, name, opts.All)
	if err != nil {
		return fmt.Errorf("survey %s: %w", name, err)
	}
	e.renderSurvey(node, opts, true)
	e.renderer.Result(output.Result{Success: true, Message: "survey " + name, Data: node})
	return nil
}

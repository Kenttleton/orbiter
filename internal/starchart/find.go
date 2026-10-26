package starchart

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Kenttleton/orbiter/internal/integrations"
)

// FindRequest is one discovery question Orbiter asks the shell.
type FindRequest struct {
	Op    string // FIND_BINARY | FIND_FILE | FIND_DIR | FIND_ENV
	Key   string // binary name, glob pattern, path, or env var name
	Value string // optional: FIND_FILE location
}

// FindResult is a parsed, path-normalized FIND response.
type FindResult struct {
	Matches []integrations.Path
	Exists  bool
	Value   string
}

// PlanFinds lists every FIND the manifest asks Orbiter to run on its behalf.
func PlanFinds(m integrations.Manifest) ([]FindRequest, error) {
	var reqs []FindRequest
	for _, b := range m.Integration.Binaries {
		reqs = append(reqs, FindRequest{Op: "FIND_BINARY", Key: b})
	}
	for _, f := range m.Find.Files {
		if strings.ContainsAny(f, " \t\n") {
			return nil, fmt.Errorf("find pattern %q must not contain whitespace", f)
		}
		reqs = append(reqs, FindRequest{Op: "FIND_FILE", Key: f})
	}
	for _, d := range m.Find.Dirs {
		reqs = append(reqs, FindRequest{Op: "FIND_DIR", Key: d})
	}
	for _, e := range m.Find.Envs {
		reqs = append(reqs, FindRequest{Op: "FIND_ENV", Key: e})
	}
	return reqs, nil
}

type findWire struct {
	Status  string   `json:"status"`
	Message string   `json:"message"`
	Matches []string `json:"matches"`
	Path    string   `json:"path"`
	Exists  bool     `json:"exists"`
	Value   string   `json:"value"`
}

// ParseFindResponse decodes the JSON a shell FIND function relayed.
// "ok" and "not_found" both succeed (empty result); "error" is an error.
func ParseFindResponse(op, raw string) (FindResult, error) {
	var w findWire
	if err := json.Unmarshal([]byte(raw), &w); err != nil {
		return FindResult{}, fmt.Errorf("%s: invalid response: %w", op, err)
	}
	switch w.Status {
	case "ok":
	case "not_found":
		return FindResult{}, nil
	case "error":
		return FindResult{}, fmt.Errorf("%s: %s", op, w.Message)
	default:
		return FindResult{}, fmt.Errorf("%s: unknown status %q", op, w.Status)
	}
	res := FindResult{Exists: w.Exists, Value: w.Value}
	for _, m := range w.Matches {
		p, err := integrations.ParsePath(m)
		if err != nil {
			return FindResult{}, fmt.Errorf("%s: bad path %q: %w", op, m, err)
		}
		res.Matches = append(res.Matches, p)
	}
	return res, nil
}

// ApplyFindResults injects one result into rc. A FIND_BINARY with more than one
// match is ambiguous and is left unset: the Captain chooses (see uplink SELECT).
func ApplyFindResults(rc *integrations.ResolvedContext, req FindRequest, res FindResult) {
	switch req.Op {
	case "FIND_BINARY":
		if len(res.Matches) != 1 {
			return
		}
		if rc.Binaries == nil {
			rc.Binaries = map[string]string{}
		}
		rc.Binaries[req.Key] = res.Matches[0].IntoNative()
	case "FIND_FILE":
		if rc.Files == nil {
			rc.Files = map[string][]string{}
		}
		paths := make([]string, 0, len(res.Matches))
		for _, m := range res.Matches {
			paths = append(paths, m.IntoNative())
		}
		rc.Files[req.Key] = paths
	case "FIND_DIR":
		if rc.Dirs == nil {
			rc.Dirs = map[string]bool{}
		}
		rc.Dirs[req.Key] = res.Exists
	case "FIND_ENV":
		if rc.Envs == nil {
			rc.Envs = map[string]string{}
		}
		rc.Envs[req.Key] = res.Value
	}
}

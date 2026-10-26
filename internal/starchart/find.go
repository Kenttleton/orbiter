package starchart

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/Kenttleton/orbiter/internal/integrations"
	"github.com/Kenttleton/orbiter/internal/models"
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

// safeBinaryName admits only plain file names. Binary names come from manifests
// (untrusted for third-party bundles) and are written into directive lines the
// shell executes, so a line break, space or path separator must never get through.
var safeBinaryName = regexp.MustCompile(`^[A-Za-z0-9._+-]+$`)

// PlanFinds lists every FIND the manifest asks Orbiter to run on its behalf.
func PlanFinds(m integrations.Manifest) ([]FindRequest, error) {
	var reqs []FindRequest
	for _, b := range m.Integration.Binaries {
		if !safeBinaryName.MatchString(b) {
			return nil, fmt.Errorf("binary name %q is not a plain file name", b)
		}
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

// recordedBinaries returns the recorded path for each declared binary,
// normalized for this host. It deliberately does not check that the path
// exists: whether the binary is still present and reachable is the
// integration's report to make. Recording happens at INIT/drift recovery (uplink).
func recordedBinaries(configJSON string, declared []string) map[string]string {
	var cfg struct {
		Binaries map[string]string `json:"binaries"`
	}
	if configJSON == "" || json.Unmarshal([]byte(configJSON), &cfg) != nil {
		return nil
	}
	out := map[string]string{}
	for _, name := range declared {
		raw, ok := cfg.Binaries[name]
		if !ok {
			continue
		}
		p, err := integrations.ParsePath(raw)
		if err != nil || p.IntoNative() == "" {
			continue
		}
		out[name] = p.IntoNative()
	}
	return out
}

// ResourceFind pairs a FIND request with the resource it is for.
type ResourceFind struct {
	ResourceID string
	Request    FindRequest
}

// NeededFinds lists a FIND_BINARY request for each binary a scanned resource's
// integration declares when either no path is recorded for it (first INIT) or the
// integration's own scan report says it is not present (it could not use the
// recorded path). Orbiter never inspects the recorded path itself. Filesystem
// resources are native and never need FIND. FIND_FILE/DIR/ENV are not requested
// here: no integration declares [find] yet.
func (sc *StarChart) NeededFinds(scan BranchScanResult) []ResourceFind {
	var out []ResourceFind
	for _, rs := range scan.Resources {
		r := rs.Resource
		if r.Role == integrations.ResourceRoleFilesystem {
			continue
		}
		integration, ok := sc.integrations.Get(r.Role, r.Brand)
		if !ok {
			continue
		}
		declared := integration.Meta().Integration.Binaries
		recorded := recordedBinaries(r.Config, declared)
		for _, name := range declared {
			if !safeBinaryName.MatchString(name) {
				continue // never turn an unsafe manifest name into a directive
			}
			if _, has := recorded[name]; has && rs.Report.Present {
				continue
			}
			out = append(out, ResourceFind{ResourceID: r.ID, Request: FindRequest{Op: "FIND_BINARY", Key: name}})
		}
	}
	return out
}

// RecordedBinary returns the path recorded for name in the resource's config
// ("" when none is recorded).
func (sc *StarChart) RecordedBinary(ctx context.Context, resourceID, name string) (string, error) {
	var r models.Resource
	if err := sc.Get(ctx, "resources", resourceID, &r); err != nil {
		return "", err
	}
	var cfg struct {
		Binaries map[string]string `json:"binaries"`
	}
	if r.Config == "" || json.Unmarshal([]byte(r.Config), &cfg) != nil {
		return "", nil
	}
	return cfg.Binaries[name], nil
}

// RecordBinary merges {"binaries":{name:path}} into the resource's config JSON,
// preserving every other key. The path is stored in forward-slash form so the
// StarChart stays portable; it is re-parsed for the current host on every read.
func (sc *StarChart) RecordBinary(ctx context.Context, resourceID, name, path string) error {
	var r models.Resource
	if err := sc.Get(ctx, "resources", resourceID, &r); err != nil {
		return err
	}
	cfg := map[string]any{}
	if r.Config != "" {
		if err := json.Unmarshal([]byte(r.Config), &cfg); err != nil {
			return fmt.Errorf("resource config is not a JSON object: %w", err)
		}
	}
	p, err := integrations.ParsePath(path)
	if err != nil {
		return err
	}
	bins, _ := cfg["binaries"].(map[string]any)
	if bins == nil {
		bins = map[string]any{}
	}
	bins[name] = p.Unix
	cfg["binaries"] = bins
	b, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	return sc.Update(ctx, "resources", resourceID, map[string]any{"config": string(b)})
}

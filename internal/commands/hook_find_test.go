package commands_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type findResp struct {
	Status  string   `json:"status"`
	Matches []string `json:"matches"`
	Exists  bool     `json:"exists"`
	Path    string   `json:"path"`
	Name    string   `json:"name"`
	Value   string   `json:"value"`
}

// runFind sources the real hook for shell, runs one FIND function and returns
// the JSON the function relayed for txn "tx1".
func runFind(t *testing.T, shell, hook, call string, env []string) findResp {
	t.Helper()
	bin, err := exec.LookPath(shell)
	if err != nil {
		t.Skipf("%s not installed", shell)
	}
	dir := t.TempDir()
	out := filepath.Join(dir, "out")
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}
	fake := filepath.Join(dir, "orbiter-fake")
	// Fake binary: `relay --txn=X` stores stdin in $OUT/X.
	script := "#!/bin/sh\ncase \"$1\" in relay) cat > \"" + out + "/${2#--txn=}\" ;; esac\n"
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	src, err := os.ReadFile(filepath.Join("..", "..", "integrations", hook))
	if err != nil {
		t.Fatal(err)
	}
	hookPath := filepath.Join(dir, filepath.Base(hook))
	body := strings.ReplaceAll(string(src), "::ORBITER::", fake)
	if err := os.WriteFile(hookPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bin, "-c", "source '"+hookPath+"'; "+call)
	cmd.Env = append(os.Environ(), env...)
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s: %v\n%s", shell, err, b)
	}
	raw, err := os.ReadFile(filepath.Join(out, "tx1"))
	if err != nil {
		t.Fatalf("nothing relayed for tx1: %v", err)
	}
	var r findResp
	if err := json.Unmarshal(raw, &r); err != nil {
		t.Fatalf("invalid JSON %q: %v", raw, err)
	}
	return r
}

func TestFindBinary_PathWalk(t *testing.T) {
	root := t.TempDir()
	a := filepath.Join(root, "a dir")   // space in path
	b := filepath.Join(root, `b"quote`) // quote in path
	c := filepath.Join(root, "c")       // holds a *directory* named like the binary
	for _, d := range []string{a, b, c} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, d := range []string{a, b} {
		if err := os.WriteFile(filepath.Join(d, "mytool"), []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(c, "mytool"), 0o755); err != nil {
		t.Fatal(err)
	}
	// duplicate entry for a, an empty entry, and the directory-only entry c
	path := strings.Join([]string{a, "", a, c, b}, ":") + ":/usr/bin:/bin"

	for _, tc := range []struct{ shell, hook string }{
		{"bash", "bash/hook.bash"},
		{"zsh", "zsh/hook.zsh"},
	} {
		t.Run(tc.shell, func(t *testing.T) {
			r := runFind(t, tc.shell, tc.hook, "FIND_BINARY tx1 mytool", []string{"PATH=" + path})
			want := []string{filepath.Join(a, "mytool"), filepath.Join(b, "mytool")}
			if r.Status != "ok" || len(r.Matches) != 2 || r.Matches[0] != want[0] || r.Matches[1] != want[1] {
				t.Errorf("got %+v, want matches %v (deduped, no dir, no empty entry)", r, want)
			}
		})
	}
}

func TestFindBinary_NoMatchIsOK(t *testing.T) {
	r := runFind(t, "bash", "bash/hook.bash", "FIND_BINARY tx1 definitely-not-a-binary", nil)
	if r.Status != "ok" || len(r.Matches) != 0 {
		t.Errorf("got %+v, want ok with no matches", r)
	}
}

func TestFindDirAndEnv(t *testing.T) {
	d := t.TempDir()
	r := runFind(t, "bash", "bash/hook.bash", "FIND_DIR tx1 '"+d+"'", nil)
	if r.Status != "ok" || !r.Exists || r.Path != d {
		t.Errorf("FIND_DIR existing: %+v", r)
	}
	r = runFind(t, "bash", "bash/hook.bash", "FIND_DIR tx1 /no/such/dir", nil)
	if r.Status != "ok" || r.Exists || r.Path != "" {
		t.Errorf("FIND_DIR missing: %+v", r)
	}
	r = runFind(t, "bash", "bash/hook.bash", "FIND_ENV tx1 ORBITER_T", []string{`ORBITER_T=va"l\ue`})
	if r.Status != "ok" || r.Name != "ORBITER_T" || r.Value != `va"l\ue` {
		t.Errorf("FIND_ENV: %+v", r)
	}
}

func TestFindFile_Recursive(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"package.json", "sub/package.json"} {
		if err := os.WriteFile(filepath.Join(root, p), []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	r := runFind(t, "bash", "bash/hook.bash", "FIND_FILE tx1 package.json '"+root+"'", nil)
	if r.Status != "ok" || len(r.Matches) != 2 {
		t.Errorf("got %+v, want 2 recursive matches", r)
	}
	r = runFind(t, "bash", "bash/hook.bash", "FIND_FILE tx1 package.json /no/such/root", nil)
	if r.Status != "not_found" {
		t.Errorf("missing root: got %+v, want not_found", r)
	}
}

// A calibrate that needs two binaries emits two FIND lines; the wrapper must run
// both, relay both, call uplink once, and print nothing it was not asked to.
func TestWrapper_TwoFindLines(t *testing.T) {
	for _, tc := range []struct{ shell, hook string }{
		{"bash", "bash/hook.bash"},
		{"zsh", "zsh/hook.zsh"},
	} {
		t.Run(tc.shell, func(t *testing.T) {
			bin, err := exec.LookPath(tc.shell)
			if err != nil {
				t.Skipf("%s not installed", tc.shell)
			}
			dir := t.TempDir()
			out := filepath.Join(dir, "out")
			if err := os.MkdirAll(out, 0o755); err != nil {
				t.Fatal(err)
			}
			fake := filepath.Join(dir, "orbiter-fake")
			script := "#!/bin/sh\ncase \"$1\" in\n" +
				"relay) cat > \"" + out + "/${2#--txn=}\" ;;\n" +
				"uplink) echo \"$3\" > \"" + out + "/uplink\" ;;\n" +
				"calibrate) printf 'SESSION ss1\\nFIND_BINARY tx1 sh\\nFIND_BINARY tx2 ls\\n' ;;\n" +
				"esac\n"
			if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
				t.Fatal(err)
			}
			src, err := os.ReadFile(filepath.Join("..", "..", "integrations", tc.hook))
			if err != nil {
				t.Fatal(err)
			}
			hookPath := filepath.Join(dir, filepath.Base(tc.hook))
			if err := os.WriteFile(hookPath, []byte(strings.ReplaceAll(string(src), "::ORBITER::", fake)), 0o644); err != nil {
				t.Fatal(err)
			}
			b, err := exec.Command(bin, "-c", "source '"+hookPath+"'; orbiter calibrate").CombinedOutput()
			if err != nil {
				t.Fatalf("%v\n%s", err, b)
			}
			if strings.TrimSpace(string(b)) != "" {
				t.Errorf("wrapper printed noise: %q", b)
			}
			for _, txn := range []string{"tx1", "tx2"} {
				if _, err := os.Stat(filepath.Join(out, txn)); err != nil {
					t.Errorf("%s was not relayed: %v", txn, err)
				}
			}
			if got, _ := os.ReadFile(filepath.Join(out, "uplink")); strings.TrimSpace(string(got)) != "ss1" {
				t.Errorf("uplink session = %q, want ss1 (the --session value)", got)
			}
		})
	}
}

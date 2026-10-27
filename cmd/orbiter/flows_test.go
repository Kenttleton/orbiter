package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rogpeppe/go-internal/testscript"
)

// TestFlows runs the end-to-end flows in testdata/flows. A flow is an ordered list of
// steps that mimics real use of Orbiter: real bundled integrations installed by
// `vessel init`, real tools on a PATH the flow controls, and a client that answers
// prompts the way a GUI would. Unit tests (go test ./internal/...) are mocked; flows
// are not.
//
// Each flow is isolated: its own work directory, HOME and PATH (setup), reported step by
// step as it runs, and torn down with a summary (teardown). Run `go test ./cmd/orbiter
// -run TestFlows -v` to see the steps, prompts and findings.
//
// Commands available to flows, beyond testscript's own:
//
//	step <title>                  log "STEP n: title"
//	finding <text>                record something the flow learned without failing it
//	client [-deny] [-select=N] [-text=V] [!] -- <orbiter args>
//	                              run `orbiter --output json -i <args>` as a client: events go
//	                              to stdout, prompts are answered (approve by default; select
//	                              option N; text V) and each answer is logged
//	capture <NAME> <regex>        set $NAME to the first group of regex in the last stdout
func TestFlows(t *testing.T) {
	// Flows run as parallel subtests, so the report is printed once they have all finished.
	t.Cleanup(func() { flowReport.print(t) })
	testscript.Run(t, testscript.Params{
		Dir:   "testdata/flows",
		Setup: flowSetup,
		Cmds: map[string]func(*testscript.TestScript, bool, []string){
			"step":    cmdStep,
			"finding": cmdFinding,
			"client":  cmdClient,
			"capture": cmdCapture,
		},
	})
}

// ---- setup and teardown ------------------------------------------------------

// flowTools are the host tools a flow may discover. They are linked into the flow's own
// bin directory, and PATH is only that directory plus the orbiter shim, so FIND sees
// exactly one of each and a flow never depends on what else is installed.
var flowTools = []string{"git"}

func flowSetup(env *testscript.Env) error {
	home := filepath.Join(env.WorkDir, "home")
	bin := filepath.Join(env.WorkDir, "bin")
	for _, d := range []string{home, bin} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	for _, tool := range flowTools {
		p, err := exec.LookPath(tool) // host PATH
		if err != nil {
			return fmt.Errorf("flow needs %s on the host PATH: %w", tool, err)
		}
		if err := os.Symlink(p, filepath.Join(bin, tool)); err != nil {
			return err
		}
	}

	// The flow plays the user's terminal with the host's real login shell (zsh on a Mac),
	// so Orbiter's own auto-detection is what finds the right shell integration. Orbiter is
	// never told which shell; the flow only sets $SHELL the way a login would.
	hostShell := os.Getenv("SHELL")
	if hostShell == "" {
		hostShell = "/bin/bash"
	}
	shellName := filepath.Base(hostShell)
	if err := os.Symlink(hostShell, filepath.Join(bin, shellName)); err != nil {
		return err
	}
	env.Setenv("SHELL", filepath.Join(bin, shellName))
	env.Setenv("FLOW_SHELL", shellName)

	// A second shell, for flows where the Captain's terminal changes. Flows that do not use
	// it ignore it; $FLOW_OTHER_SHELL names it and $FLOW_OTHER_SHELL_PATH is where it is linked.
	for _, candidate := range []string{"bash", "zsh", "fish"} {
		if candidate == shellName {
			continue
		}
		p, err := exec.LookPath(candidate)
		if err != nil {
			continue
		}
		if err := os.Symlink(p, filepath.Join(bin, candidate)); err != nil {
			return err
		}
		env.Setenv("FLOW_OTHER_SHELL", candidate)
		env.Setenv("FLOW_OTHER_SHELL_PATH", filepath.Join(bin, candidate))
		break
	}

	shim := ""
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		if _, err := os.Stat(filepath.Join(dir, "orbiter")); err == nil {
			shim = dir
			break
		}
	}
	if shim == "" {
		return fmt.Errorf("orbiter shim not found on PATH")
	}
	env.Setenv("HOME", home)
	env.Setenv("PATH", shim+string(os.PathListSeparator)+bin)
	env.Setenv("ORBITER_APPROVAL_TIMEOUT", "20s") // a flow that forgets to answer fails fast
	env.Setenv("GIT_CONFIG_NOSYSTEM", "1")

	flowReport.start(env.WorkDir, env.T().(interface{ Name() string }).Name())
	env.Defer(func() { flowReport.finish(env.WorkDir) })
	return nil
}

// ---- reporting ---------------------------------------------------------------

type report struct {
	mu       sync.Mutex
	names    map[string]string
	steps    map[string]int
	findings map[string][]string
	lines    []string
}

var flowReport = &report{names: map[string]string{}, steps: map[string]int{}, findings: map[string][]string{}}

func (r *report) start(work, name string) {
	r.mu.Lock()
	r.names[work] = name
	r.steps[work] = 0
	r.mu.Unlock()
}

func (r *report) step(work string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.steps[work]++
	return r.steps[work]
}

func (r *report) addFinding(work, text string) {
	r.mu.Lock()
	r.findings[work] = append(r.findings[work], text)
	r.mu.Unlock()
}

// finish is the teardown summary for one flow.
func (r *report) finish(work string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	line := fmt.Sprintf("%s: %d steps, %d findings", r.names[work], r.steps[work], len(r.findings[work]))
	r.lines = append(r.lines, line)
	for _, f := range r.findings[work] {
		r.lines = append(r.lines, "    finding: "+f)
	}
}

func (r *report) print(t *testing.T) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.lines) == 0 {
		return
	}
	t.Log("\n== flow report ==\n" + strings.Join(r.lines, "\n"))
}

// lockedBuffer collects a process's stderr while the client is still reading stdout.
type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

// clientIdleTimeout is how long the client waits for orbiter to say something (an event,
// or exit) before it fails the step. A stall almost always means orbiter is waiting for
// input the client cannot see, which is itself a finding worth reporting.
const clientIdleTimeout = 15 * time.Second

// ---- commands ----------------------------------------------------------------

func cmdStep(ts *testscript.TestScript, neg bool, args []string) {
	if neg {
		ts.Fatalf("unsupported: ! step")
	}
	ts.Logf("STEP %d: %s", flowReport.step(ts.Getenv("WORK")), strings.Join(args, " "))
}

func cmdFinding(ts *testscript.TestScript, neg bool, args []string) {
	if neg {
		ts.Fatalf("unsupported: ! finding")
	}
	text := strings.Join(args, " ")
	flowReport.addFinding(ts.Getenv("WORK"), text)
	ts.Logf("FINDING: %s", text)
}

func cmdCapture(ts *testscript.TestScript, neg bool, args []string) {
	if neg || len(args) != 2 {
		ts.Fatalf("usage: capture NAME REGEX")
	}
	re, err := regexp.Compile(args[1])
	if err != nil {
		ts.Fatalf("capture: %v", err)
	}
	m := re.FindStringSubmatch(ts.ReadFile("stdout"))
	if len(m) < 2 {
		ts.Fatalf("capture %s: no match for %q in stdout", args[0], args[1])
	}
	ts.Setenv(args[0], m[1])
	ts.Logf("captured %s=%s", args[0], m[1])
}

// cmdClient runs orbiter attached (-i) and answers its prompts like a GUI would.
func cmdClient(ts *testscript.TestScript, neg bool, args []string) {
	approve, selectN, text := true, 1, ""
	rest := args
loop:
	for len(rest) > 0 {
		switch a := rest[0]; {
		case a == "--":
			rest = rest[1:]
			break loop
		case a == "-deny":
			approve = false
		case strings.HasPrefix(a, "-select="):
			selectN, _ = strconv.Atoi(strings.TrimPrefix(a, "-select="))
		case strings.HasPrefix(a, "-text="):
			text = strings.TrimPrefix(a, "-text=")
		default:
			ts.Fatalf("client: unknown option %q", a)
		}
		rest = rest[1:]
	}

	exe := ""
	for _, dir := range filepath.SplitList(ts.Getenv("PATH")) {
		if _, err := os.Stat(filepath.Join(dir, "orbiter")); err == nil {
			exe = filepath.Join(dir, "orbiter")
			break
		}
	}
	if exe == "" {
		ts.Fatalf("client: orbiter not found on PATH")
	}

	cmd := exec.Command(exe, append([]string{"--output", "json", "-i"}, rest...)...)
	cmd.Dir = ts.MkAbs(".")
	cmd.Env = []string{
		"HOME=" + ts.Getenv("HOME"), "PATH=" + ts.Getenv("PATH"),
		"ORBITER_APPROVAL_TIMEOUT=" + ts.Getenv("ORBITER_APPROVAL_TIMEOUT"),
		"GIT_CONFIG_NOSYSTEM=1", "TERM=dumb", "SHELL=" + ts.Getenv("SHELL"),
	}
	stderr := &lockedBuffer{}
	cmd.Stderr = stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		ts.Fatalf("client: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		ts.Fatalf("client: %v", err)
	}
	if err := cmd.Start(); err != nil {
		ts.Fatalf("client: %v", err)
	}

	out := ts.Stdout()
	lines := make(chan string)
	go func() {
		sc := bufio.NewScanner(stdout)
		sc.Buffer(make([]byte, 0, 1<<20), 1<<20)
		for sc.Scan() {
			lines <- sc.Text()
		}
		close(lines)
	}()
reading:
	for {
		var line string
		select {
		case l, ok := <-lines:
			if !ok {
				break reading
			}
			line = l
		case <-time.After(clientIdleTimeout):
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			ts.Fatalf("client: orbiter said nothing for %v and is probably waiting for input the client cannot see.\nstderr so far:\n%s", clientIdleTimeout, stderr.String())
		}
		fmt.Fprintln(out, line)
		var e struct {
			Type string          `json:"type"`
			Txn  string          `json:"txn"`
			Data json.RawMessage `json:"data"`
		}
		if json.Unmarshal([]byte(line), &e) != nil || e.Type != "captain_prompt" {
			continue
		}
		var p struct {
			Op      string   `json:"op"`
			Message string   `json:"message"`
			Options []string `json:"options"`
		}
		_ = json.Unmarshal(e.Data, &p)
		var response, shown string
		switch p.Op {
		case "CONFIRM":
			response, shown = fmt.Sprintf(`{"confirmed":%v}`, approve), map[bool]string{true: "approved", false: "denied"}[approve]
		case "SELECT":
			if selectN < 1 || selectN > len(p.Options) {
				ts.Fatalf("client: select=%d but the prompt offered %d options", selectN, len(p.Options))
			}
			b, _ := json.Marshal(map[string]string{"selected": p.Options[selectN-1]})
			response, shown = string(b), "selected "+p.Options[selectN-1]
		case "TEXT":
			b, _ := json.Marshal(map[string]string{"value": text})
			response, shown = string(b), "text answered"
		default:
			ts.Fatalf("client: unknown prompt op %q", p.Op)
		}
		ts.Logf("  prompt %s %q -> %s", p.Op, p.Message, shown)
		fmt.Fprintf(stdin, `{"txn":%q,"response":%s}`+"\n", e.Txn, response)
	}
	_ = stdin.Close()
	werr := cmd.Wait()
	io.WriteString(ts.Stderr(), stderr.String())
	switch {
	case neg && werr == nil:
		ts.Fatalf("client: command succeeded unexpectedly")
	case !neg && werr != nil:
		ts.Fatalf("client: %v\n%s", werr, stderr.String())
	}
}

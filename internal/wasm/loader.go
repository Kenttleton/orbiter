package wasm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Kenttleton/orbiter/internal/integrations"
	"github.com/Kenttleton/orbiter/internal/models"
	"github.com/Kenttleton/orbiter/internal/output"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

// ApproveFunc is called when a WASM integration attempts a command that has not
// been previously allowed. It must print a prompt and return true if the Captain
// approves this run, or false to deny.
type ApproveFunc func(brand, fullCmd string) bool

// StdinApproveFunc is the default ApproveFunc, used when running interactively.
func StdinApproveFunc(brand, fullCmd string) bool {
	fmt.Fprintf(os.Stderr, "\n  orbiter: integration %q wants to run:\n    %s\n  Allow? [y/N] ", brand, display(fullCmd))
	var response string
	fmt.Fscan(os.Stdin, &response)
	return strings.ToLower(strings.TrimSpace(response)) == "y"
}

// AlwaysAllowFunc is called after Captain approval to ask whether this exact
// command should always be allowed. Returns true to persist the trust entry.
type AlwaysAllowFunc func(brand, fullCmd string) bool

// PromptStore persists a prompt as a transaction so the Captain's answer can arrive
// from another process (orbiter relay). *starchart.StarChart implements it.
type PromptStore interface {
	// OpenSession starts a session and returns its ID.
	OpenSession(ctx context.Context, meta string) (string, error)
	// RecordPrompt saves a pending transaction for a prompt (payload is the prompt JSON).
	RecordPrompt(ctx context.Context, sessionID, txn, op, payload string) error
	// PromptResponse returns the relayed response once the transaction has one.
	PromptResponse(ctx context.Context, txn string) (response string, answered bool, err error)
	// CloseTransaction marks the transaction processed.
	CloseTransaction(ctx context.Context, txn string) error
}

// ApprovalTimeout bounds how long an unanswered asynchronous approval holds an
// integration call open before it is denied. ORBITER_APPROVAL_TIMEOUT overrides it.
var ApprovalTimeout = 5 * time.Minute

// ApprovalPollInterval is how often a waiting approval checks for a relayed answer.
var ApprovalPollInterval = 200 * time.Millisecond

// RendererApproveFunc returns an ApproveFunc that asks the Captain through the
// renderer. A prompt is a transaction: it is recorded, shown (the captain_prompt
// event carries its txn), and answered one of two ways, both available to clients:
//
//   - Sync: the renderer returns the answer itself (styled mode reading the terminal,
//     or JSON mode attached with -i reading a line from stdin).
//   - Async: the renderer returns no answer (JSON mode without -i). The call stays
//     open while the client answers with `orbiter relay --txn=<txn>` from any process;
//     the answer is read back from the store. No answer within ApprovalTimeout denies.
//
// A nil store means the answer cannot be awaited, so an unanswered prompt denies.
func RendererApproveFunc(r output.Renderer, store PromptStore) ApproveFunc {
	c := &confirmer{r: r, store: store}
	return func(brand, fullCmd string) bool {
		return c.ask(fmt.Sprintf("%s wants to run: %s", display(brand), display(fullCmd)))
	}
}

// display makes text chosen by a guest or a manifest safe to show the Captain: control characters
// (escape sequences, carriage returns, newlines) are written out as \x1b, \r, \n instead of being
// sent to the terminal, where they could redraw the prompt to say something else.
func display(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == 0x7f || (r < 0x20) || (r >= 0x80 && r < 0xa0):
			fmt.Fprintf(&b, "\\x%02x", r)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// RendererAlwaysAllowFunc returns an AlwaysAllowFunc with the same transaction flow as
// RendererApproveFunc. An unanswered prompt does not grant a permanent allow.
func RendererAlwaysAllowFunc(r output.Renderer, store PromptStore) AlwaysAllowFunc {
	c := &confirmer{r: r, store: store}
	return func(brand, fullCmd string) bool {
		return c.ask(fmt.Sprintf("Always allow this exact command for %q? %s", display(brand), display(fullCmd)))
	}
}

// Prompts are the two questions the Captain is asked about a command an integration
// wants to run: whether to run it now, and whether to trust it from then on.
type Prompts struct {
	Approve     ApproveFunc     // nil: StdinApproveFunc
	AlwaysAllow AlwaysAllowFunc // nil: StdinAlwaysAllowFunc
}

// RendererPrompts returns both questions asked through the renderer, sharing one
// session so the two prompts about a command belong together.
func RendererPrompts(r output.Renderer, store PromptStore) Prompts {
	c := &confirmer{r: r, store: store}
	return Prompts{
		Approve: func(brand, fullCmd string) bool {
			return c.ask(fmt.Sprintf("%s wants to run: %s", display(brand), display(fullCmd)))
		},
		AlwaysAllow: func(brand, fullCmd string) bool {
			return c.ask(fmt.Sprintf("Always allow this exact command for %q? %s", display(brand), display(fullCmd)))
		},
	}
}

// confirmer asks CONFIRM prompts through a renderer and a store, one session per confirmer.
type confirmer struct {
	r       output.Renderer
	store   PromptStore
	once    sync.Once
	session string
	openErr error
}

// ask records the prompt, shows it, and returns the Captain's answer (false when none).
func (c *confirmer) ask(message string) bool {
	ctx := context.Background()
	prompt := output.CaptainPrompt{Txn: models.NewID("tx"), Op: "CONFIRM", Message: message}
	recorded := false
	if c.store != nil {
		c.once.Do(func() { c.session, c.openErr = c.store.OpenSession(ctx, `{"op":"approval"}`) })
		if c.openErr == nil {
			payload, _ := json.Marshal(prompt)
			recorded = c.store.RecordPrompt(ctx, c.session, prompt.Txn, prompt.Op, string(payload)) == nil
		}
	}
	resp, err := c.r.Prompt(prompt)
	if err != nil {
		return false
	}
	if resp == "" && recorded {
		resp = awaitRelayedResponse(ctx, c.store, prompt.Txn)
	}
	if recorded {
		_ = c.store.CloseTransaction(ctx, prompt.Txn)
	}
	if resp == "" {
		return false
	}
	var result struct {
		Confirmed bool `json:"confirmed"`
	}
	if err := json.Unmarshal([]byte(resp), &result); err != nil {
		return false
	}
	return result.Confirmed
}

// awaitRelayedResponse polls the store until the transaction has a response or
// ApprovalTimeout passes. It returns "" on timeout.
func awaitRelayedResponse(ctx context.Context, store PromptStore, txn string) string {
	deadline := time.Now().Add(ApprovalTimeout)
	for time.Now().Before(deadline) {
		if resp, ok, err := store.PromptResponse(ctx, txn); err == nil && ok {
			return resp
		}
		time.Sleep(ApprovalPollInterval)
	}
	return ""
}

// StdinAlwaysAllowFunc is the default AlwaysAllowFunc.
func StdinAlwaysAllowFunc(brand, fullCmd string) bool {
	fmt.Fprintf(os.Stderr, "  Always allow this exact command for %q? [y/N] ", brand)
	var response string
	fmt.Fscan(os.Stdin, &response)
	return strings.ToLower(strings.TrimSpace(response)) == "y"
}

// WASMIntegration is a compiled WASM module and implements integrations.Integration. It is
// stateless and serverless-style: every call (detect, initialize, scan, calibrate) instantiates a
// fresh module, runs the export and closes the instance, so nothing one call leaves in memory
// reaches the next. Orbiter is the state manager; an integration keeps nothing. At most
// manifest.Runtime.PoolSize instances run at once (default 1).
type WASMIntegration struct {
	manifest    integrations.Manifest
	slots       chan struct{} // one token per instance that may be alive at once (manifest pool_size)
	alive       atomic.Int32
	rt          wazero.Runtime
	compiled    wazero.CompiledModule
	approve     ApproveFunc
	alwaysAllow AlwaysAllowFunc
	settings    *integrations.SettingsStore
	registry    *integrations.Registry
}

// Load compiles wasmBytes, checks that the module imports nothing the host does not offer, and
// returns a WASMIntegration ready to serve calls. No instance is created until a call needs one.
// A nil field in prompts falls back to reading the terminal.
func Load(ctx context.Context, manifest integrations.Manifest, wasmBytes []byte, settings *integrations.SettingsStore, registry *integrations.Registry, prompts Prompts) (*WASMIntegration, error) {
	approve, alwaysAllow := prompts.Approve, prompts.AlwaysAllow
	if approve == nil {
		approve = StdinApproveFunc
	}
	if alwaysAllow == nil {
		alwaysAllow = StdinAlwaysAllowFunc
	}

	if err := manifest.Runtime.ValidateEngine(); err != nil {
		return nil, err
	}
	if err := manifest.Runtime.ValidateMemory(); err != nil {
		return nil, err
	}
	rt, err := RuntimeFor(ctx, manifest.Runtime.EngineName(), manifest.Runtime.MemoryPages())
	if err != nil {
		return nil, err
	}

	compiled, err := rt.CompileModule(ctx, wasmBytes)
	if err != nil {
		return nil, fmt.Errorf("compile wasm module: %w", err)
	}

	if err := checkImports(compiled); err != nil {
		compiled.Close(ctx)
		return nil, err
	}

	poolSize := manifest.Runtime.PoolSize
	if poolSize <= 0 {
		poolSize = 1
	}
	slots := make(chan struct{}, poolSize)
	for range poolSize {
		slots <- struct{}{}
	}

	return &WASMIntegration{
		manifest:    manifest,
		slots:       slots,
		rt:          rt,
		compiled:    compiled,
		approve:     approve,
		alwaysAllow: alwaysAllow,
		settings:    settings,
		registry:    registry,
	}, nil
}

// instantiate makes one live instance of a compiled module. The guest gets no files, no console
// and no clock beyond what wazero's defaults give a module with no WASI: only the orbiter host
// functions.
func instantiate(ctx context.Context, rt wazero.Runtime, compiled wazero.CompiledModule) (api.Module, error) {
	return rt.InstantiateModule(ctx, compiled,
		wazero.NewModuleConfig().
			WithName("").
			WithStdout(io.Discard).
			WithStderr(io.Discard),
	)
}

// live is how many instances are alive right now.
func (w *WASMIntegration) live() int { return int(w.alive.Load()) }

// hostImports is every function a guest may import: the orbiter host functions, and the abort
// hook AssemblyScript modules always carry. Anything else (WASI, a function that does not exist)
// is a guest asking for more than it was given.
var hostImports = map[string]bool{
	"orbiter.read_input":   true,
	"orbiter.write_output": true,
	"orbiter.run_command":  true,
	"env.abort":            true,
}

// checkImports rejects a module that imports anything outside hostImports, memories and tables
// included.
func checkImports(compiled wazero.CompiledModule) error {
	for _, f := range compiled.ImportedFunctions() {
		mod, name, _ := f.Import()
		if !hostImports[mod+"."+name] {
			return fmt.Errorf("guest imports %s.%s, which the host does not offer", mod, name)
		}
	}
	if len(compiled.ImportedMemories()) > 0 {
		return fmt.Errorf("guest imports a memory; a guest owns its own")
	}
	return nil
}

func (w *WASMIntegration) Meta() integrations.Manifest { return w.manifest }

// Engine is the engine this integration's wasm runs on: interpreter or compiler.
func (w *WASMIntegration) Engine() string { return w.manifest.Runtime.EngineName() }

func (w *WASMIntegration) Detect(ctx integrations.DetectContext) integrations.DetectReport {
	input, _ := json.Marshal(ctx)
	out, err := w.invoke(context.Background(), "detect", input)
	if err != nil {
		return integrations.DetectReport{}
	}
	var report integrations.DetectReport
	if err := json.Unmarshal(out, &report); err != nil {
		return integrations.DetectReport{}
	}
	return report
}

func (w *WASMIntegration) Init(ctx integrations.ResolvedContext) integrations.StateReport {
	input, _ := json.Marshal(ctx)
	out, err := w.invoke(context.Background(), "initialize", input)
	if err != nil {
		return integrations.StateReport{Error: err.Error()}
	}
	var report integrations.StateReport
	if err := json.Unmarshal(out, &report); err != nil {
		return integrations.StateReport{Error: fmt.Sprintf("unmarshal response: %v", err)}
	}
	report = FilterExports(report, w.manifest.Shell.AllowedEnvs())
	return report
}

func (w *WASMIntegration) Scan(ctx integrations.ResolvedContext) integrations.StateReport {
	input, _ := json.Marshal(ctx)
	out, err := w.invoke(context.Background(), "scan", input)
	if err != nil {
		return integrations.StateReport{Error: err.Error()}
	}
	var report integrations.StateReport
	if err := json.Unmarshal(out, &report); err != nil {
		return integrations.StateReport{Error: fmt.Sprintf("unmarshal response: %v", err)}
	}
	report = FilterExports(report, w.manifest.Shell.AllowedEnvs())
	return report
}

func (w *WASMIntegration) Calibrate(ctx integrations.ResolvedContext) integrations.StateReport {
	input, _ := json.Marshal(ctx)
	out, err := w.invoke(context.Background(), "calibrate", input)
	if err != nil {
		return integrations.StateReport{Error: err.Error()}
	}
	var report integrations.StateReport
	if err := json.Unmarshal(out, &report); err != nil {
		return integrations.StateReport{Error: fmt.Sprintf("unmarshal response: %v", err)}
	}
	report = FilterExports(report, w.manifest.Shell.AllowedEnvs())
	return report
}

// invoke checks out a module from the pool, calls fn, then returns the module.
// callState is threaded through context so host functions can read/write the
// JSON payload without any shared mutable state on the struct.
// On error the module is closed and discarded rather than returned to the pool,
// because a WASM trap may leave the module in a corrupted state.
func (w *WASMIntegration) invoke(ctx context.Context, fn string, input []byte) (_ []byte, retErr error) {
	select {
	case <-w.slots:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { w.slots <- struct{}{} }()

	mod, err := instantiate(ctx, w.rt, w.compiled)
	if err != nil {
		return nil, fmt.Errorf("instantiate: %w", err)
	}
	w.alive.Add(1)
	defer func() {
		mod.Close(context.Background())
		w.alive.Add(-1)
	}()

	cs := &callState{
		input:       input,
		brand:       w.manifest.Integration.Brand,
		allowed:     w.manifest.Commands.AllowedCmds(),
		commands:    w.manifest.Commands,
		timeout:     w.manifest.Commands.TimeoutSeconds,
		approve:     w.approve,
		alwaysAllow: w.alwaysAllow,
		settings:    w.settings,
		registry:    w.registry,
	}
	ctx = context.WithValue(ctx, callStateKey{}, cs)

	// The guest's compute budget: stop the call when the guest has used GuestTimeout of its own time.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	start := time.Now()
	timedOut := make(chan struct{})
	go func() {
		tick := time.NewTicker(max(GuestTimeout/20, time.Millisecond))
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
				if cs.guestElapsed(start) > GuestTimeout {
					close(timedOut)
					cancel()
					return
				}
			}
		}
	}()

	exported := mod.ExportedFunction(fn)
	if exported == nil {
		return nil, fmt.Errorf("function %q not exported by wasm module", fn)
	}
	if _, err := exported.Call(ctx); err != nil {
		select {
		case <-timedOut:
			return nil, fmt.Errorf("call %q: guest used more than %s of compute and was stopped", fn, GuestTimeout)
		default:
		}
		return nil, fmt.Errorf("call %q: %w", fn, err)
	}
	return cs.output, nil
}

// FilterExports removes entries from report.Exports whose keys are not in
// the manifest's Shell.Exports allowlist. If the allowlist is empty, all
// exports are stripped. Returns a shallow copy of report with the filtered map.
func FilterExports(report integrations.StateReport, allowed []string) integrations.StateReport {
	if report.Exports == nil {
		return report
	}
	if len(allowed) == 0 {
		report.Exports = nil
		return report
	}
	allowSet := make(map[string]struct{}, len(allowed))
	for _, k := range allowed {
		allowSet[k] = struct{}{}
	}
	filtered := make(map[string]string)
	for k, v := range report.Exports {
		if _, ok := allowSet[k]; ok {
			filtered[k] = v
		}
	}
	report.Exports = filtered
	return report
}

var _ integrations.Integration = (*WASMIntegration)(nil)

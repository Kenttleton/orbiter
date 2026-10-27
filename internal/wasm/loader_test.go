package wasm_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Kenttleton/orbiter/internal/output"
	"github.com/Kenttleton/orbiter/internal/wasm"
)

func TestRendererApproveFunc_confirm_yes(t *testing.T) {
	var buf bytes.Buffer
	r := output.NewStyledRendererWith(&buf, strings.NewReader("y\n"), false)
	fn := wasm.RendererApproveFunc(r, nil)
	if !fn("git", "git status") {
		t.Error("expected approve = true for 'y' input")
	}
}

func TestRendererApproveFunc_confirm_no(t *testing.T) {
	var buf bytes.Buffer
	r := output.NewStyledRendererWith(&buf, strings.NewReader("n\n"), false)
	fn := wasm.RendererApproveFunc(r, nil)
	if fn("git", "git status") {
		t.Error("expected approve = false for 'n' input")
	}
}

func TestRendererApproveFunc_json_mode_without_a_store_denies(t *testing.T) {
	var buf bytes.Buffer
	r := output.NewJSONRenderer(&buf, false)
	fn := wasm.RendererApproveFunc(r, nil)
	// JSON mode: Prompt emits event and returns ("", nil).
	// RendererApproveFunc must treat empty response as denial.
	if fn("git", "git status") {
		t.Error("JSON mode: empty response should be treated as denial")
	}
	// The captain_prompt event was emitted.
	if buf.Len() == 0 {
		t.Error("JSON mode: expected captain_prompt event on stdout")
	}
}

// memStore is an in-memory PromptStore: relay stands in for `orbiter relay`.
type memStore struct {
	mu        sync.Mutex
	sessions  int
	prompts   map[string]string // txn -> payload
	responses map[string]string // txn -> relayed response
	closed    map[string]bool
	recorded  chan string // txn of each recorded prompt
}

func newMemStore() *memStore {
	return &memStore{prompts: map[string]string{}, responses: map[string]string{},
		closed: map[string]bool{}, recorded: make(chan string, 8)}
}

func (m *memStore) OpenSession(context.Context, string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions++
	return "session1", nil
}

func (m *memStore) RecordPrompt(_ context.Context, _, txn, _, payload string) error {
	m.mu.Lock()
	m.prompts[txn] = payload
	m.mu.Unlock()
	m.recorded <- txn
	return nil
}

func (m *memStore) PromptResponse(_ context.Context, txn string) (string, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.responses[txn]
	return r, ok, nil
}

func (m *memStore) CloseTransaction(_ context.Context, txn string) error {
	m.mu.Lock()
	m.closed[txn] = true
	m.mu.Unlock()
	return nil
}

func (m *memStore) relay(txn, response string) {
	m.mu.Lock()
	m.responses[txn] = response
	m.mu.Unlock()
}

func fastApproval(t *testing.T, timeout time.Duration) {
	t.Helper()
	oldT, oldP := wasm.ApprovalTimeout, wasm.ApprovalPollInterval
	wasm.ApprovalTimeout, wasm.ApprovalPollInterval = timeout, 5*time.Millisecond
	t.Cleanup(func() { wasm.ApprovalTimeout, wasm.ApprovalPollInterval = oldT, oldP })
}

// Async: the process holds the call open and the client answers with orbiter relay
// (here, relay on the shared store) using the txn from the captain_prompt event.
func TestRendererApproveFunc_async_waits_for_relay(t *testing.T) {
	fastApproval(t, 5*time.Second)
	store := newMemStore()
	var buf bytes.Buffer
	fn := wasm.RendererApproveFunc(output.NewJSONRenderer(&buf, false), store)

	go func() {
		txn := <-store.recorded
		time.Sleep(30 * time.Millisecond)
		store.relay(txn, `{"confirmed":true}`)
	}()
	if !fn("git", "git status") {
		t.Fatal("approval relayed as confirmed must allow the command")
	}
	if !strings.Contains(buf.String(), `"captain_prompt"`) {
		t.Errorf("the prompt event must be emitted: %q", buf.String())
	}
	if store.sessions != 1 {
		t.Errorf("sessions = %d, want 1", store.sessions)
	}
	var recordedTxn string
	for txn := range store.closed {
		recordedTxn = txn
	}
	if recordedTxn == "" || !strings.Contains(store.prompts[recordedTxn], "git status") {
		t.Errorf("the prompt must be recorded and the transaction closed: %v", store.closed)
	}
}

func TestRendererApproveFunc_async_declined(t *testing.T) {
	fastApproval(t, 5*time.Second)
	store := newMemStore()
	fn := wasm.RendererApproveFunc(output.NewJSONRenderer(&bytes.Buffer{}, false), store)
	go func() { store.relay(<-store.recorded, `{"confirmed":false}`) }()
	if fn("git", "git status") {
		t.Fatal("a declined approval must deny")
	}
}

func TestRendererApproveFunc_async_timeout_denies(t *testing.T) {
	fastApproval(t, 80*time.Millisecond)
	fn := wasm.RendererApproveFunc(output.NewJSONRenderer(&bytes.Buffer{}, false), newMemStore())
	start := time.Now()
	if fn("git", "git status") {
		t.Fatal("no answer within the timeout must deny")
	}
	if time.Since(start) > 2*time.Second {
		t.Errorf("timeout took %v", time.Since(start))
	}
}

// Sync (-i): the client answers on the attached stdin, no relay needed.
func TestRendererApproveFunc_attached_answers_on_stdin(t *testing.T) {
	fastApproval(t, 5*time.Second)
	store := newMemStore()
	stdinR, stdinW := io.Pipe()
	stdoutR, stdoutW := io.Pipe()
	r := output.NewJSONRenderer(stdoutW, false)
	r.Attach(stdinR)
	fn := wasm.RendererApproveFunc(r, store)

	go func() { // the client: read the prompt event, answer with its txn
		line, _ := bufio.NewReader(stdoutR).ReadString('\n')
		var e output.Event
		_ = json.Unmarshal([]byte(line), &e)
		io.WriteString(stdinW, `{"txn":"`+e.Txn+`","response":{"confirmed":true}}`+"\n")
	}()
	if !fn("git", "git status") {
		t.Fatal("answer on stdin must allow the command")
	}
}

func TestRendererApproveFunc_attached_closed_stdin_denies_at_once(t *testing.T) {
	fastApproval(t, 5*time.Second)
	r := output.NewJSONRenderer(&bytes.Buffer{}, false)
	r.Attach(strings.NewReader(""))
	fn := wasm.RendererApproveFunc(r, newMemStore())
	start := time.Now()
	if fn("git", "git status") {
		t.Fatal("a client that closed stdin cannot approve")
	}
	if time.Since(start) > time.Second {
		t.Errorf("must not wait out the timeout, took %v", time.Since(start))
	}
}

// "Always allow this exact command?" is the second question asked about a command an
// integration wants to run. It is a prompt like any other: tagged with a txn, answered
// on the attached stdin (sync) or through relay (async), never read from raw stdin.
func TestRendererAlwaysAllowFunc_attached_answers_on_stdin(t *testing.T) {
	fastApproval(t, 5*time.Second)
	stdinR, stdinW := io.Pipe()
	stdoutR, stdoutW := io.Pipe()
	r := output.NewJSONRenderer(stdoutW, false)
	r.Attach(stdinR)
	fn := wasm.RendererAlwaysAllowFunc(r, newMemStore())

	var message string
	go func() {
		line, _ := bufio.NewReader(stdoutR).ReadString('\n')
		var e struct {
			Txn  string `json:"txn"`
			Data struct {
				Message string `json:"message"`
			} `json:"data"`
		}
		_ = json.Unmarshal([]byte(line), &e)
		message = e.Data.Message
		io.WriteString(stdinW, `{"txn":"`+e.Txn+`","response":{"confirmed":true}}`+"\n")
	}()
	if !fn("git", "git --version") {
		t.Fatal("a confirmed answer must allow always")
	}
	if !strings.Contains(message, "Always allow") || !strings.Contains(message, "git --version") {
		t.Errorf("the prompt must say what is being trusted, got %q", message)
	}
}

func TestRendererAlwaysAllowFunc_async_relay_and_timeout(t *testing.T) {
	fastApproval(t, 80*time.Millisecond)
	store := newMemStore()
	fn := wasm.RendererAlwaysAllowFunc(output.NewJSONRenderer(&bytes.Buffer{}, false), store)
	go func() { store.relay(<-store.recorded, `{"confirmed":true}`) }()
	oldT := wasm.ApprovalTimeout
	wasm.ApprovalTimeout = 5 * time.Second
	if !fn("git", "git --version") {
		t.Fatal("a relayed confirmation must allow always")
	}
	wasm.ApprovalTimeout = oldT
	if fn("git", "git --version") {
		t.Fatal("no answer within the timeout must not grant a permanent allow")
	}
}

// Both questions about one command belong to one session.
func TestRendererPrompts_share_one_session(t *testing.T) {
	fastApproval(t, 5*time.Second)
	store := newMemStore()
	p := wasm.RendererPrompts(output.NewJSONRenderer(&bytes.Buffer{}, false), store)
	go func() {
		for i := 0; i < 2; i++ {
			store.relay(<-store.recorded, `{"confirmed":true}`)
		}
	}()
	if !p.Approve("git", "git --version") || !p.AlwaysAllow("git", "git --version") {
		t.Fatal("both relayed confirmations must be honored")
	}
	if store.sessions != 1 {
		t.Errorf("sessions = %d, want 1 shared by both prompts", store.sessions)
	}
}

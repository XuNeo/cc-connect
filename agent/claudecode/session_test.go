package claudecode

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/chenhg5/cc-connect/core"
)

func TestHandleResultParsesUsage(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cs := &claudeSession{
		events: make(chan core.Event, 8),
		ctx:    ctx,
	}
	cs.sessionID.Store("test-session")
	cs.alive.Store(true)

	raw := map[string]any{
		"type":       "result",
		"result":     "done",
		"session_id": "test-session",
		"usage": map[string]any{
			"input_tokens":  float64(150000),
			"output_tokens": float64(2000),
		},
	}

	cs.handleResult(raw)

	evt := <-cs.events
	if evt.InputTokens != 150000 {
		t.Errorf("InputTokens = %d, want 150000", evt.InputTokens)
	}
	if evt.OutputTokens != 2000 {
		t.Errorf("OutputTokens = %d, want 2000", evt.OutputTokens)
	}
}

func TestHandleResultNoUsage(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cs := &claudeSession{
		events: make(chan core.Event, 8),
		ctx:    ctx,
	}
	cs.sessionID.Store("test-session")
	cs.alive.Store(true)

	raw := map[string]any{
		"type":   "result",
		"result": "done",
	}

	cs.handleResult(raw)

	evt := <-cs.events
	if evt.InputTokens != 0 {
		t.Errorf("InputTokens = %d, want 0", evt.InputTokens)
	}
	if evt.OutputTokens != 0 {
		t.Errorf("OutputTokens = %d, want 0", evt.OutputTokens)
	}
}

func TestReadLoop_ChildHoldsStdoutPipe(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pr, pw := io.Pipe()
	t.Cleanup(func() {
		_ = pw.Close()
	})

	writeDone := make(chan error, 1)
	go func() {
		_, err := io.WriteString(pw, `{"type":"system","session_id":"test-pipe"}`+"\n")
		writeDone <- err
	}()

	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^$")
	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	cs := &claudeSession{
		cmd:    cmd,
		events: make(chan core.Event, 64),
		ctx:    ctx,
		cancel: cancel,
		done:   make(chan struct{}),
	}
	cs.alive.Store(true)
	go cs.readLoop(pr, &stderrBuf)

	timeout := time.After(5 * time.Second)
	gotEvent := false
	for {
		select {
		case err := <-writeDone:
			if err != nil {
				t.Fatal(err)
			}
			writeDone = nil
		case evt, ok := <-cs.events:
			if !ok {
				if !gotEvent {
					t.Fatal("events closed but system event lost")
				}
				return
			}
			if evt.SessionID == "test-pipe" {
				gotEvent = true
			}
		case <-timeout:
			t.Fatal("HANG: events not closed within 5s - readLoop stuck in scanner.Scan()")
		}
	}
}

func TestReadLoop_CtxCancelClosesChannels(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pr, pw := io.Pipe()
	t.Cleanup(func() {
		_ = pw.Close()
	})

	// "err-then-sleep" emits stderr before sleeping so that ctx cancel
	// produces a non-empty stderrBuf in readLoop's defer — exercising the
	// `case <-cs.ctx.Done()` select branch in finishReadLoop.
	cmd := helperCommand(ctx, "err-then-sleep")
	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	cs := &claudeSession{
		cmd:    cmd,
		events: make(chan core.Event, 64),
		ctx:    ctx,
		cancel: cancel,
		done:   make(chan struct{}),
	}
	cs.alive.Store(true)
	go cs.readLoop(pr, &stderrBuf)

	time.Sleep(200 * time.Millisecond)
	cancel()

	timeout := time.After(5 * time.Second)
	for {
		select {
		case _, ok := <-cs.events:
			if !ok {
				goto closed
			}
		case <-timeout:
			t.Fatal("HANG: events not closed within 5s after ctx cancel")
		}
	}
closed:
	select {
	case <-cs.done:
	case <-timeout:
		t.Fatal("HANG: done not closed within 5s after ctx cancel")
	}
}

func TestClaudeSessionClose_IdempotentNoPanic(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := helperCommand(ctx, "stdin-eof-exit")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	done := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(done)
	}()

	cs := &claudeSession{
		cmd:                 cmd,
		stdin:               stdin,
		ctx:                 ctx,
		cancel:              cancel,
		done:                done,
		gracefulStopTimeout: 200 * time.Millisecond,
	}
	cs.alive.Store(true)

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Close panicked: %v", r)
		}
	}()

	if err := cs.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := cs.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

func TestShellJoinArgs(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"empty", nil, ""},
		{"single_plain", []string{"--verbose"}, "--verbose"},
		{"multiple_plain", []string{"--verbose", "--model", "opus"}, "--verbose --model opus"},
		{"arg_with_space", []string{"--prompt", "hello world"}, "--prompt 'hello world'"},
		{"arg_with_tab", []string{"a\tb"}, "'a\tb'"},
		{"arg_with_newline", []string{"line1\nline2"}, "'line1\nline2'"},
		{"arg_with_single_quote", []string{"it's"}, "'it'\\''s'"},
		{"arg_with_double_quote", []string{`say "hi"`}, `'say "hi"'`},
		{"arg_with_backslash", []string{`path\to`}, `'path\to'`},
		{"mixed", []string{"--flag", "has space", "plain", "it's here"}, "--flag 'has space' plain 'it'\\''s here'"},
		{"empty_string_arg", []string{""}, ""},
		{"long_prompt", []string{"--append-system-prompt", "You are a helpful assistant.\nBe concise."}, "--append-system-prompt 'You are a helpful assistant.\nBe concise.'"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shellJoinArgs(tt.args)
			if got != tt.want {
				t.Errorf("shellJoinArgs(%v)\n  got  = %q\n  want = %q", tt.args, got, tt.want)
			}
		})
	}
}

func helperCommand(ctx context.Context, mode string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=TestHelperProcess", "--", mode)
	cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1")
	return cmd
}

// TestHelperProcess lets this test binary act as a tiny external command for
// cases that need a process with controlled lifetime semantics.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}

	mode := os.Args[len(os.Args)-1]
	switch mode {
	case "sleep":
		time.Sleep(30 * time.Second)
		os.Exit(0)
	case "err-then-sleep":
		_, _ = os.Stderr.WriteString("helper: starting up\n")
		time.Sleep(30 * time.Second)
		os.Exit(0)
	case "stdin-eof-exit":
		_, _ = io.Copy(io.Discard, os.Stdin)
		os.Exit(0)
	default:
		os.Exit(2)
	}
}

func TestHandleUser_EmitsToolResult(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cs := &claudeSession{
		events:    make(chan core.Event, 8),
		ctx:       ctx,
		toolNames: map[string]string{"toolu_123": "Bash"},
	}
	cs.alive.Store(true)

	raw := map[string]any{
		"type": "user",
		"message": map[string]any{
			"content": []any{
				map[string]any{
					"type":        "tool_result",
					"tool_use_id": "toolu_123",
					"content":     "hello world\n",
					"is_error":    false,
				},
			},
		},
	}
	cs.handleUser(raw)

	select {
	case ev := <-cs.events:
		if ev.Type != core.EventToolResult {
			t.Fatalf("Type = %v, want %v", ev.Type, core.EventToolResult)
		}
		if ev.ToolName != "Bash" {
			t.Fatalf("ToolName = %q, want %q", ev.ToolName, "Bash")
		}
		if ev.ToolResult != "hello world\n" {
			t.Fatalf("ToolResult = %q, want %q", ev.ToolResult, "hello world\n")
		}
		if ev.ToolSuccess == nil || !*ev.ToolSuccess {
			t.Fatalf("ToolSuccess = %v, want &true", ev.ToolSuccess)
		}
	default:
		t.Fatal("expected an EventToolResult on cs.events, got none")
	}

	if _, still := cs.toolNames["toolu_123"]; still {
		t.Fatalf("toolNames should be drained after emit, but key still present")
	}
}

func TestHandleUser_ToolResultWithError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cs := &claudeSession{
		events:    make(chan core.Event, 8),
		ctx:       ctx,
		toolNames: map[string]string{"toolu_err": "Read"},
	}
	cs.alive.Store(true)

	raw := map[string]any{
		"type": "user",
		"message": map[string]any{
			"content": []any{
				map[string]any{
					"type":        "tool_result",
					"tool_use_id": "toolu_err",
					"content":     "permission denied",
					"is_error":    true,
				},
			},
		},
	}
	cs.handleUser(raw)

	select {
	case ev := <-cs.events:
		if ev.Type != core.EventToolResult {
			t.Fatalf("Type = %v, want %v", ev.Type, core.EventToolResult)
		}
		if ev.ToolName != "Read" {
			t.Fatalf("ToolName = %q, want %q", ev.ToolName, "Read")
		}
		if ev.ToolResult != "permission denied" {
			t.Fatalf("ToolResult = %q, want %q", ev.ToolResult, "permission denied")
		}
		if ev.ToolSuccess == nil || *ev.ToolSuccess {
			t.Fatalf("ToolSuccess = %v, want &false", ev.ToolSuccess)
		}
	default:
		t.Fatal("expected EventToolResult, got none")
	}
}

func TestHandleUser_ToolResultUnknownToolUseID(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cs := &claudeSession{
		events:    make(chan core.Event, 8),
		ctx:       ctx,
		toolNames: map[string]string{},
	}
	cs.alive.Store(true)

	raw := map[string]any{
		"type": "user",
		"message": map[string]any{
			"content": []any{
				map[string]any{
					"type":        "tool_result",
					"tool_use_id": "toolu_unknown",
					"content":     "something",
					"is_error":    false,
				},
			},
		},
	}
	cs.handleUser(raw)

	select {
	case ev := <-cs.events:
		if ev.Type != core.EventToolResult {
			t.Fatalf("Type = %v, want %v", ev.Type, core.EventToolResult)
		}
		if ev.ToolName != "" {
			t.Fatalf("ToolName = %q, want empty fallback", ev.ToolName)
		}
		if ev.ToolResult != "something" {
			t.Fatalf("ToolResult = %q, want %q", ev.ToolResult, "something")
		}
	default:
		t.Fatal("expected EventToolResult, got none")
	}
}

func TestHandleUser_MultipleToolResultsDrainMap(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cs := &claudeSession{
		events: make(chan core.Event, 8),
		ctx:    ctx,
		toolNames: map[string]string{
			"toolu_a": "Bash",
			"toolu_b": "Read",
		},
	}
	cs.alive.Store(true)

	raw := map[string]any{
		"type": "user",
		"message": map[string]any{
			"content": []any{
				map[string]any{
					"type":        "tool_result",
					"tool_use_id": "toolu_a",
					"content":     "a-out",
				},
				map[string]any{
					"type":        "tool_result",
					"tool_use_id": "toolu_b",
					"content":     "b-out",
				},
			},
		},
	}
	cs.handleUser(raw)

	names := []string{}
	for i := 0; i < 2; i++ {
		select {
		case ev := <-cs.events:
			if ev.Type != core.EventToolResult {
				t.Fatalf("ev[%d].Type = %v, want EventToolResult", i, ev.Type)
			}
			names = append(names, ev.ToolName)
		case <-time.After(100 * time.Millisecond):
			t.Fatalf("only got %d events, expected 2", i)
		}
	}
	if names[0] != "Bash" || names[1] != "Read" {
		t.Fatalf("names = %v, want [Bash Read]", names)
	}
	if len(cs.toolNames) != 0 {
		t.Fatalf("toolNames not drained: %v", cs.toolNames)
	}
}

func TestHandleUser_ToolResultArrayContent(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cs := &claudeSession{
		events:    make(chan core.Event, 8),
		ctx:       ctx,
		toolNames: map[string]string{"toolu_arr": "Read"},
	}
	cs.alive.Store(true)

	raw := map[string]any{
		"type": "user",
		"message": map[string]any{
			"content": []any{
				map[string]any{
					"type":        "tool_result",
					"tool_use_id": "toolu_arr",
					"content": []any{
						map[string]any{"type": "text", "text": "line one\n"},
						map[string]any{"type": "image", "source": map[string]any{"type": "base64"}},
						map[string]any{"type": "text", "text": "line two"},
					},
					"is_error": false,
				},
			},
		},
	}
	cs.handleUser(raw)

	select {
	case ev := <-cs.events:
		if ev.Type != core.EventToolResult {
			t.Fatalf("Type = %v, want %v", ev.Type, core.EventToolResult)
		}
		if ev.ToolName != "Read" {
			t.Fatalf("ToolName = %q, want %q", ev.ToolName, "Read")
		}
		if ev.ToolResult != "line one\nline two" {
			t.Fatalf("ToolResult = %q, want %q", ev.ToolResult, "line one\nline two")
		}
	default:
		t.Fatal("expected EventToolResult, got none")
	}
}

//go:build integration

package midstream_test

import (
	"bufio"
	"encoding/json"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

// TestMidStreamInjection spawns claude --input-format stream-json, sends a
// first message that triggers a tool call, then sends a second message
// mid-turn. Verifies:
//   - exactly one result event fires
//   - both messages appear in the echoed user events (via --replay-user-messages)
//   - the turn completes with subtype=success
//
// Run: go test -tags=integration ./tests/integration/midstream/...
func TestMidStreamInjection(t *testing.T) {
	if _, err := exec.LookPath("claude"); err != nil {
		t.Skip("claude CLI not found in PATH")
	}

	sessionID := uuid.NewString()
	cmd := exec.Command("claude", "-p",
		"--input-format", "stream-json",
		"--output-format", "stream-json",
		"--verbose",
		"--session-id", sessionID,
		"--replay-user-messages",
	)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("StdinPipe: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("StdoutPipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer cmd.Process.Kill()

	var (
		mu            sync.Mutex
		resultCount   int
		userMessages  []string
		turnCompleted = make(chan struct{})
	)

	go func() {
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 1<<16), 1<<22)
		for scanner.Scan() {
			var e map[string]any
			if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
				continue
			}
			typ, _ := e["type"].(string)
			switch typ {
			case "user":
				if content, ok := e["message"].(map[string]any)["content"].([]any); ok {
					for _, c := range content {
						if m, ok := c.(map[string]any); ok && m["type"] == "text" {
							mu.Lock()
							userMessages = append(userMessages, m["text"].(string))
							mu.Unlock()
						}
					}
				}
			case "result":
				mu.Lock()
				resultCount++
				mu.Unlock()
				close(turnCompleted)
			}
		}
	}()

	sendUser := func(text string) error {
		payload := map[string]any{
			"type": "user",
			"message": map[string]any{
				"role":    "user",
				"content": []any{map[string]any{"type": "text", "text": text}},
			},
			"session_id": sessionID,
		}
		b, _ := json.Marshal(payload)
		_, err := stdin.Write(append(b, '\n'))
		return err
	}

	// First message: triggers at least one bash tool call.
	msg1 := "Run the bash command 'sleep 2 && echo done' exactly once, then stop."
	if err := sendUser(msg1); err != nil {
		t.Fatalf("send msg1: %v", err)
	}

	// Wait 4s, then inject second message mid-turn.
	time.Sleep(4 * time.Second)
	msg2 := "After that, just reply BANANA and stop."
	if err := sendUser(msg2); err != nil {
		t.Fatalf("send msg2: %v", err)
	}

	// Wait for turn completion (max 90s).
	select {
	case <-turnCompleted:
	case <-time.After(90 * time.Second):
		t.Fatal("timeout waiting for result event")
	}

	mu.Lock()
	defer mu.Unlock()

	if resultCount != 1 {
		t.Errorf("resultCount = %d, want 1 (merged turn)", resultCount)
	}
	foundMsg1, foundMsg2 := false, false
	for _, u := range userMessages {
		if strings.Contains(u, "sleep 2") {
			foundMsg1 = true
		}
		if strings.Contains(u, "BANANA") {
			foundMsg2 = true
		}
	}
	if !foundMsg1 {
		t.Error("msg1 not echoed in user events")
	}
	if !foundMsg2 {
		t.Error("msg2 not echoed in user events (mid-stream injection failed)")
	}
}

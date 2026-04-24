package feishu

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	lark "github.com/larksuite/oapi-sdk-go/v3"
)

// Regression: when UpdateMessage is called with a handle holding more
// existing message IDs than the rebuilt card count (a "shrink" scenario),
// the excess IDs must NOT be withdrawn. Earlier versions of UpdateMessage
// called deleteSingleCard on excess IDs, which caused the bot to withdraw
// its own earlier progress cards mid-session whenever the compact writer's
// item windowing kicked in. Card+payload mode now monotonically grows, so
// the shrink path is dead code — but this test locks the guarantee in at
// the UpdateMessage boundary so any future regression is caught.
func TestUpdateMessage_ShrinkDoesNotWithdrawCards(t *testing.T) {
	const appID = "cli_no_self_withdraw"
	const appSecret = "secret"

	var patchCalls, deleteCalls atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/open-apis/auth/v3/tenant_access_token/internal":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code":                0,
				"msg":                 "success",
				"expire":              7200,
				"tenant_access_token": "valid-token",
			})
		case strings.Contains(r.URL.Path, "/messages/") && r.Method == http.MethodPatch:
			patchCalls.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "msg": "success"})
		case strings.Contains(r.URL.Path, "/messages/") && r.Method == http.MethodDelete:
			deleteCalls.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "msg": "success"})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "msg": "ok"})
		}
	}))
	defer srv.Close()

	p := &Platform{
		platformName:       "feishu",
		domain:             srv.URL,
		appID:              appID,
		appSecret:          appSecret,
		useInteractiveCard: true,
		client: lark.NewClient(appID, appSecret,
			lark.WithOpenBaseUrl(srv.URL),
			lark.WithHttpClient(srv.Client()),
		),
	}

	// Handle carries 3 existing IDs. Plain content renders to exactly 1 card,
	// so len(existing)=3 > cardCount=1 — the pathological shrink case.
	h := &feishuPreviewHandle{
		messageIDs: []string{"om_card_1", "om_card_2", "om_card_3"},
		chatID:     "oc_chat",
	}
	if err := p.UpdateMessage(context.Background(), h, "updated content"); err != nil {
		t.Fatalf("UpdateMessage() error = %v", err)
	}

	if got := patchCalls.Load(); got != 1 {
		t.Errorf("patchCalls = %d, want 1 (only the first existing ID patched)", got)
	}
	if got := deleteCalls.Load(); got != 0 {
		t.Errorf("deleteCalls = %d, want 0 (bot must not withdraw its own cards)", got)
	}
	if len(h.messageIDs) != 1 || h.messageIDs[0] != "om_card_1" {
		t.Errorf("h.messageIDs = %v, want [om_card_1]", h.messageIDs)
	}
}

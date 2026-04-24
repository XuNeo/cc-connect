package feishu

import (
	"strings"
	"testing"

	core "github.com/chenhg5/cc-connect/core"
)

func TestFreshCardNotice_ZhAndEn(t *testing.T) {
	zhPayload := &core.ProgressCardPayload{Lang: "zh", State: core.ProgressCardStateRunning}
	got := freshCardNotice(zhPayload)
	if got == "" {
		t.Fatal("zh notice empty")
	}
	if !strings.Contains(got, "续期") && !strings.Contains(got, "新卡") {
		t.Errorf("zh notice missing '续期'/'新卡': %q", got)
	}

	enPayload := &core.ProgressCardPayload{Lang: "en"}
	got = freshCardNotice(enPayload)
	if got == "" {
		t.Fatal("en notice empty")
	}
	if !strings.Contains(got, "continued") && !strings.Contains(got, "renewed") && !strings.Contains(got, "new card") {
		t.Errorf("en notice missing 'continued'/'renewed'/'new card': %q", got)
	}

	// nil payload should still produce a non-empty (default-en) notice.
	if got := freshCardNotice(nil); got == "" {
		t.Error("nil payload produced empty notice")
	}
}

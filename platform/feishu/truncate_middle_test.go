package feishu

import (
	"strings"
	"testing"
)

func TestTruncateMiddle_ShortUnchanged(t *testing.T) {
	in := "hello world"
	got := truncateMiddle(in, 100)
	if got != in {
		t.Errorf("got %q, want %q", got, in)
	}
}

func TestTruncateMiddle_CutsMiddleAndAnnotates(t *testing.T) {
	in := strings.Repeat("a", 400) + strings.Repeat("b", 400)
	got := truncateMiddle(in, 200)
	if !strings.Contains(got, "...") {
		t.Errorf("missing ellipsis marker: %q", got[:80])
	}
	if !strings.HasPrefix(got, "a") {
		t.Errorf("want head kept, got prefix %q", got[:10])
	}
	if !strings.HasSuffix(got, "b") {
		t.Errorf("want tail kept, got suffix %q", got[len(got)-10:])
	}
	// Must include omitted byte count for data-loss transparency.
	if !strings.Contains(got, "omitted") && !strings.Contains(got, "省略") {
		t.Errorf("missing omitted-byte annotation: %q", got)
	}
}

func TestTruncateMiddle_UTF8Safe(t *testing.T) {
	in := strings.Repeat("中", 500) + strings.Repeat("文", 500)
	got := truncateMiddle(in, 300)
	if !strings.Contains(got, "中") || !strings.Contains(got, "文") {
		t.Errorf("dropped UTF-8 boundaries: %q", got)
	}
}

func TestTruncateMiddle_ExactBoundary(t *testing.T) {
	in := strings.Repeat("x", 200)
	got := truncateMiddle(in, 200)
	if got != in {
		t.Errorf("should not truncate at exact cap")
	}
}

func TestTruncateMiddle_TinyMaxRunesReportsActualOmitted(t *testing.T) {
	// 200 runes, maxRunes=5 → fallback floor fires, kept=40, omitted=160.
	in := strings.Repeat("a", 200)
	got := truncateMiddle(in, 5)
	if !strings.Contains(got, "omitted") {
		t.Fatalf("missing annotation: %q", got)
	}
	// The annotation must report the TRUE omitted count (total - kept),
	// not total - maxRunes. Writing a tiny maxRunes forced the fallback
	// path so the old computation (total-maxRunes=195) now differs from
	// the true omitted count (total-40=160).
	if !strings.Contains(got, "160") {
		t.Errorf("omitted count should be 160 (200 total - 40 kept), got annotation in %q", got)
	}
	if strings.Contains(got, "195") {
		t.Errorf("omitted count must not be 195 (= total - maxRunes); got %q", got)
	}
}

func TestTruncateMiddle_ChineseTinyReportsActualOmitted(t *testing.T) {
	// Chinese path, fallback triggered.
	in := strings.Repeat("中", 200)
	got := truncateMiddle(in, 5)
	if !strings.Contains(got, "省略") {
		t.Fatalf("missing zh annotation: %q", got)
	}
	if !strings.Contains(got, "160") {
		t.Errorf("zh omitted count should be 160, got %q", got)
	}
}

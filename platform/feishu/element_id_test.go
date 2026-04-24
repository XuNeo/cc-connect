package feishu

import (
	"regexp"
	"testing"
)

var elementIDPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,19}$`)

func TestMakeElementID_SatisfiesFeishuRules(t *testing.T) {
	cases := []struct{ prefix string; seq int }{
		{"t", 0},
		{"t", 9999},
		{"think", 1},
		{"", 42},
		{"weird-name!!", 3},
	}
	for _, c := range cases {
		id := makeElementID(c.prefix, c.seq)
		if !elementIDPattern.MatchString(id) {
			t.Errorf("id=%q does not match Feishu rule for prefix=%q seq=%d", id, c.prefix, c.seq)
		}
		if len(id) > 20 {
			t.Errorf("id=%q length %d > 20", id, len(id))
		}
	}
}

func TestMakeElementID_StableForSameInputs(t *testing.T) {
	a := makeElementID("bash", 7)
	b := makeElementID("bash", 7)
	if a != b {
		t.Errorf("not deterministic: %q vs %q", a, b)
	}
}

func TestMakeElementID_DifferentSeqDifferentID(t *testing.T) {
	if makeElementID("bash", 1) == makeElementID("bash", 2) {
		t.Error("same id for different seq")
	}
}

func TestMakeElementID_LargeSeqNoCollision(t *testing.T) {
	// Guard against silent truncation producing colliding IDs for
	// different seq values. With a long prefix and 7-digit seq, the
	// naive "maxPrefix = 13" budget would truncate the seq digits.
	a := makeElementID("toolname1234", 9_999_999)
	b := makeElementID("toolname1234", 9_999_998)
	if a == b {
		t.Errorf("different seq values produced identical IDs: %q == %q", a, b)
	}
	if !elementIDPattern.MatchString(a) {
		t.Errorf("large-seq id=%q does not match Feishu rule", a)
	}
	if len(a) > 20 {
		t.Errorf("id=%q length %d > 20", a, len(a))
	}
}

func TestMakeElementID_HugeSeqStillValid(t *testing.T) {
	// Pathological seq (19 digits — max int64 is 19 chars). Should still
	// produce a 20-char, letter-start id.
	id := makeElementID("t", 1_000_000_000_000_000_000)
	if !elementIDPattern.MatchString(id) {
		t.Errorf("huge-seq id=%q does not match Feishu rule", id)
	}
	if len(id) > 20 {
		t.Errorf("id=%q length %d > 20", id, len(id))
	}
}

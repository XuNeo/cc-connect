package feishu

import "testing"

func TestPlanCardUpdates_SameLength(t *testing.T) {
	p, s := planCardUpdates([]string{"a", "b"}, 2)
	if p != 2 || s != 0 {
		t.Errorf("patch=%d send=%d", p, s)
	}
}

func TestPlanCardUpdates_Grow(t *testing.T) {
	p, s := planCardUpdates([]string{"a"}, 3)
	if p != 1 || s != 2 {
		t.Errorf("patch=%d send=%d", p, s)
	}
}

// Shrinking input used to return a toDelete list that UpdateMessage passed
// to deleteSingleCard — which caused the bot to withdraw its own earlier
// progress cards mid-session. card+payload rendering now grows monotonically,
// so planCardUpdates only caps patchCount at cardCount and never emits work
// on the excess. This test locks that in: even if callers pass a pathological
// existing>cardCount, sendCount stays 0 and patchCount clamps to cardCount —
// no signal to delete.
func TestPlanCardUpdates_ShrinkIsNoop(t *testing.T) {
	p, s := planCardUpdates([]string{"a", "b", "c"}, 1)
	if p != 1 || s != 0 {
		t.Errorf("patch=%d send=%d", p, s)
	}
}

func TestPlanCardUpdates_Empty(t *testing.T) {
	p, s := planCardUpdates(nil, 2)
	if p != 0 || s != 2 {
		t.Errorf("patch=%d send=%d", p, s)
	}
}

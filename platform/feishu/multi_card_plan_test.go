package feishu

import "testing"

func TestPlanCardUpdates_SameLength(t *testing.T) {
	p, s, d := planCardUpdates([]string{"a", "b"}, 2)
	if p != 2 || s != 0 || len(d) != 0 {
		t.Errorf("patch=%d send=%d del=%v", p, s, d)
	}
}

func TestPlanCardUpdates_Grow(t *testing.T) {
	p, s, d := planCardUpdates([]string{"a"}, 3)
	if p != 1 || s != 2 || len(d) != 0 {
		t.Errorf("patch=%d send=%d del=%v", p, s, d)
	}
}

func TestPlanCardUpdates_Shrink(t *testing.T) {
	p, s, d := planCardUpdates([]string{"a", "b", "c"}, 1)
	if p != 1 || s != 0 || len(d) != 2 || d[0] != "b" || d[1] != "c" {
		t.Errorf("patch=%d send=%d del=%v", p, s, d)
	}
}

func TestPlanCardUpdates_Empty(t *testing.T) {
	p, s, d := planCardUpdates(nil, 2)
	if p != 0 || s != 2 || len(d) != 0 {
		t.Errorf("patch=%d send=%d del=%v", p, s, d)
	}
}

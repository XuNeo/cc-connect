package feishu

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPaginateElements_FitsInOnePage(t *testing.T) {
	els := make([]map[string]any, 5)
	for i := range els {
		els[i] = map[string]any{"tag": "markdown", "content": "hi"}
	}
	pages := paginateElements(els, 150, 18_000)
	if len(pages) != 1 {
		t.Errorf("want 1 page, got %d", len(pages))
	}
	if len(pages[0]) != 5 {
		t.Errorf("want 5 elements in page, got %d", len(pages[0]))
	}
}

func TestPaginateElements_SplitsByElementCount(t *testing.T) {
	els := make([]map[string]any, 400)
	for i := range els {
		els[i] = map[string]any{"tag": "markdown", "content": "x"}
	}
	pages := paginateElements(els, 150, 1_000_000)
	if len(pages) < 3 {
		t.Errorf("want >=3 pages for 400 items @150/page, got %d", len(pages))
	}
	total := 0
	for _, p := range pages {
		if len(p) > 150 {
			t.Errorf("page size %d > budget 150", len(p))
		}
		total += len(p)
	}
	if total != 400 {
		t.Errorf("sum of pages %d != 400", total)
	}
}

func TestPaginateElements_SplitsByJSONBytes(t *testing.T) {
	big := strings.Repeat("A", 5_000)
	els := make([]map[string]any, 20)
	for i := range els {
		els[i] = map[string]any{"tag": "markdown", "content": big}
	}
	pages := paginateElements(els, 1000, 18_000)
	if len(pages) < 2 {
		t.Errorf("expected multiple pages due to byte budget, got %d", len(pages))
	}
	for i, p := range pages {
		b, _ := json.Marshal(p)
		if len(b) > 20_000 {
			t.Errorf("page %d is %d bytes, > 20000", i, len(b))
		}
	}
}

func TestPaginateElements_SingleHugeElementForced(t *testing.T) {
	huge := strings.Repeat("Z", 30_000)
	els := []map[string]any{{"tag": "markdown", "content": huge}}
	pages := paginateElements(els, 150, 18_000)
	if len(pages) != 1 || len(pages[0]) != 1 {
		t.Errorf("expect single-element page, got %d pages (%v)", len(pages), pages)
	}
}

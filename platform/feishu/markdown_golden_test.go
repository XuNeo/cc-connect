package feishu

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var updateGolden = flag.Bool("update-golden", false, "regenerate markdown golden files instead of comparing")

// TestMarkdownGolden snapshots the output of cc-connect's two markdown
// transformers — preprocessFeishuMarkdown (the only step that mutates the
// raw markdown string before Feishu renders it) and buildPostJSON (the
// post-format fallback). It is a baseline before any parser-based rewrite.
//
// First run: `go test ./platform/feishu/ -run TestMarkdownGolden -update-golden`
// to populate testdata/markdown_golden/*.golden.
//
// Subsequent runs (without -update-golden) fail on any diff so refactors
// can show their behavioral footprint exactly.
func TestMarkdownGolden(t *testing.T) {
	dir := "testdata/markdown_golden"
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".md") {
			continue
		}
		base := strings.TrimSuffix(name, ".md")
		t.Run(base, func(t *testing.T) {
			input, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				t.Fatal(err)
			}
			src := string(input)

			preprocessed := preprocessFeishuMarkdown(src)

			postJSON := buildPostJSON(src)
			var pretty bytes.Buffer
			if err := json.Indent(&pretty, []byte(postJSON), "", "  "); err != nil {
				t.Fatalf("indent post JSON: %v", err)
			}

			preGolden := filepath.Join(dir, base+".preprocess.golden")
			postGolden := filepath.Join(dir, base+".post.golden")

			if *updateGolden {
				if err := os.WriteFile(preGolden, []byte(preprocessed), 0644); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(postGolden, pretty.Bytes(), 0644); err != nil {
					t.Fatal(err)
				}
				return
			}

			want, err := os.ReadFile(preGolden)
			if err != nil {
				t.Fatalf("missing %s — run with -update-golden to create", preGolden)
			}
			if string(want) != preprocessed {
				t.Errorf("preprocess golden mismatch for %s\n--- want ---\n%s\n--- got ---\n%s", base, want, preprocessed)
			}

			want, err = os.ReadFile(postGolden)
			if err != nil {
				t.Fatalf("missing %s — run with -update-golden to create", postGolden)
			}
			if string(want) != pretty.String() {
				t.Errorf("post golden mismatch for %s\n--- want ---\n%s\n--- got ---\n%s", base, want, pretty.String())
			}
		})
	}
}

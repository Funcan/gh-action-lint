package lint

import (
	"testing"
)

func TestSplitUses(t *testing.T) {
	tests := []struct {
		uses                      string
		owner, repo, subpath, ref string
	}{
		{
			uses:  "actions/checkout@v4",
			owner: "actions", repo: "checkout", subpath: "", ref: "v4",
		},
		{
			uses:  "github/codeql-action/analyze@abc123",
			owner: "github", repo: "codeql-action", subpath: "analyze", ref: "abc123",
		},
		{
			uses:  "no-at-sign",
			owner: "", repo: "", subpath: "", ref: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.uses, func(t *testing.T) {
			owner, repo, subpath, ref := splitUses(tt.uses)
			if owner != tt.owner || repo != tt.repo || subpath != tt.subpath || ref != tt.ref {
				t.Errorf("splitUses(%q) = (%q,%q,%q,%q), want (%q,%q,%q,%q)",
					tt.uses, owner, repo, subpath, ref,
					tt.owner, tt.repo, tt.subpath, tt.ref)
			}
		})
	}
}

func TestExternalUsesFromFile(t *testing.T) {
	content := []byte(`
name: CI
on: push
permissions: {}
jobs:
  build:
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@11bd317f7bc71dd3eee3f1bf1c58bc03de17e433
      - uses: ./local-action
      - uses: docker://alpine:3.18
`)
	ws, uses, err := parseContent(content, "ci.yml", DisabledChecks{})
	if err != nil {
		t.Fatal(err)
	}
	// Only the tag-pinned one should warn
	if len(ws) != 1 || ws[0].Uses != "actions/checkout@v4" {
		t.Errorf("unexpected warnings: %v", ws)
	}
	// Both external (non-local, non-docker) refs should be in uses
	if len(uses) != 2 {
		t.Errorf("expected 2 external uses, got %d: %v", len(uses), uses)
	}
}

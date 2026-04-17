package lint

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCheckUses(t *testing.T) {
	tests := []struct {
		uses    string
		wantNil bool
	}{
		{"actions/checkout@v4", false},                                      // tag
		{"actions/checkout@main", false},                                    // branch
		{"actions/checkout@11bd317f7bc71dd3eee3f1bf1c58bc03de17e433", true}, // SHA - ok
		{"actions/checkout", false},                                         // no ref
		{"./local-action", true},                                            // local - ok
		{"docker://alpine:3.18", true},                                      // docker - ok
	}

	for _, tt := range tests {
		t.Run(tt.uses, func(t *testing.T) {
			got := checkUses(tt.uses, "test.yml", 1)
			if tt.wantNil && got != nil {
				t.Errorf("expected no warning for %q, got one", tt.uses)
			}
			if !tt.wantNil && got == nil {
				t.Errorf("expected warning for %q, got none", tt.uses)
			}
		})
	}
}

func TestCheckFile(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "ci.yml")
	content := `
name: CI
on: push
permissions: {}
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@11bd317f7bc71dd3eee3f1bf1c58bc03de17e433
`
	if err := os.WriteFile(f, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	warnings, err := CheckFile(f, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d", len(warnings))
	}
	if warnings[0].Uses != "actions/checkout@v4" {
		t.Errorf("unexpected uses value: %s", warnings[0].Uses)
	}
}

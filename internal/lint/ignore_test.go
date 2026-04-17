package lint

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsIgnored(t *testing.T) {
	il := &IgnoreList{patterns: map[string]bool{
		"actions/checkout":      true,
		"actions/cache@v3":      true,
	}}

	tests := []struct {
		uses string
		want bool
	}{
		{"actions/checkout@v4", true},                                         // matched by name
		{"actions/checkout@main", true},                                       // matched by name
		{"actions/checkout@11bd317f7bc71dd3eee3f1bf1c58bc03de17e433", true},  // matched by name
		{"actions/cache@v3", true},                                            // exact ref match
		{"actions/cache@v2", false},                                           // different ref, no name match
		{"actions/setup-go@v4", false},                                        // not in list
	}

	for _, tt := range tests {
		t.Run(tt.uses, func(t *testing.T) {
			if got := il.IsIgnored(tt.uses); got != tt.want {
				t.Errorf("IsIgnored(%q) = %v, want %v", tt.uses, got, tt.want)
			}
		})
	}
}

func TestLoadIgnoreFile(t *testing.T) {
	dir := t.TempDir()
	content := `
# This action is considered safe
actions/checkout

actions/cache@v3   # only this specific version

# blank lines above are fine
`
	if err := os.WriteFile(filepath.Join(dir, ".gh-lint-ignore"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	il, err := LoadIgnoreFile(dir)
	if err != nil {
		t.Fatal(err)
	}

	if !il.IsIgnored("actions/checkout@v4") {
		t.Error("expected actions/checkout@v4 to be ignored")
	}
	if !il.IsIgnored("actions/cache@v3") {
		t.Error("expected actions/cache@v3 to be ignored")
	}
	if il.IsIgnored("actions/cache@v2") {
		t.Error("expected actions/cache@v2 NOT to be ignored")
	}
}

func TestLoadIgnoreFileMissing(t *testing.T) {
	il, err := LoadIgnoreFile(t.TempDir())
	if err != nil {
		t.Fatalf("expected no error for missing file, got: %v", err)
	}
	if il.IsIgnored("actions/checkout@v4") {
		t.Error("empty ignore list should not ignore anything")
	}
}

func TestCheckFileWithIgnore(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "ci.yml")
	content := `
name: CI
on: push
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/cache@v3
      - uses: actions/setup-go@11bd317f7bc71dd3eee3f1bf1c58bc03de17e433
`
	if err := os.WriteFile(f, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	il := &IgnoreList{patterns: map[string]bool{"actions/checkout": true}}
	warnings, err := CheckFile(f, il)
	if err != nil {
		t.Fatal(err)
	}

	// checkout is ignored; cache@v3 should still warn
	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d: %v", len(warnings), warnings)
	}
	if warnings[0].Uses != "actions/cache@v3" {
		t.Errorf("unexpected warning: %s", warnings[0].Uses)
	}
}

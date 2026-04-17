package lint

import (
	"os"
	"path/filepath"
	"strings"
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

func TestCheckFile_DisabledChecks(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "ci.yml")
	// Triggers all three checks: unpinned action, script injection, missing permissions.
	content := `
name: CI
on: [push, issues]
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - run: echo "${{ github.event.issue.title }}"
`
	if err := os.WriteFile(f, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	countByKind := func(ws []Warning) (pins, injections, permissions int) {
		for _, w := range ws {
			switch {
			case strings.Contains(w.Message, "not pinned"):
				pins++
			case strings.Contains(w.Message, "script injection"):
				injections++
			case strings.Contains(w.Message, "permissions"):
				permissions++
			}
		}
		return
	}

	// All enabled: expect one of each.
	ws, err := CheckFile(f, nil, DisabledChecks{})
	if err != nil {
		t.Fatal(err)
	}
	if p, i, perm := countByKind(ws); p != 1 || i != 1 || perm != 1 {
		t.Errorf("all enabled: want 1/1/1 (pins/injections/permissions), got %d/%d/%d", p, i, perm)
	}

	// Disable pins.
	ws, _ = CheckFile(f, nil, DisabledChecks{Pins: true})
	if p, i, perm := countByKind(ws); p != 0 || i != 1 || perm != 1 {
		t.Errorf("pins disabled: want 0/1/1, got %d/%d/%d", p, i, perm)
	}

	// Disable injections.
	ws, _ = CheckFile(f, nil, DisabledChecks{Injections: true})
	if p, i, perm := countByKind(ws); p != 1 || i != 0 || perm != 1 {
		t.Errorf("injections disabled: want 1/0/1, got %d/%d/%d", p, i, perm)
	}

	// Disable permissions.
	ws, _ = CheckFile(f, nil, DisabledChecks{Permissions: true})
	if p, i, perm := countByKind(ws); p != 1 || i != 1 || perm != 0 {
		t.Errorf("permissions disabled: want 1/1/0, got %d/%d/%d", p, i, perm)
	}

	// Disable all.
	ws, _ = CheckFile(f, nil, DisabledChecks{Pins: true, Injections: true, Permissions: true})
	if len(ws) != 0 {
		t.Errorf("all disabled: expected no warnings, got %d: %v", len(ws), ws)
	}
}

// filterWarnings must not suppress injection or permissions warnings even when
// the ignore list would match their (empty) Uses field.
func TestFilterWarnings_NonPinWarningsPassThrough(t *testing.T) {
	il := &IgnoreList{patterns: map[string]bool{
		"actions/checkout": true,
	}}
	warnings := []Warning{
		{File: "ci.yml", Line: 1, Uses: "", Message: "no top-level permissions declared"},
		{File: "ci.yml", Line: 5, Uses: "", Message: "script injection: ${{ github.event.issue.title }}"},
		{File: "ci.yml", Line: 8, Uses: "actions/checkout@v4", Message: "action not pinned to a SHA: actions/checkout@v4"},
	}
	got := filterWarnings(warnings, il)
	// The pin warning is ignored; the other two (empty Uses) must survive.
	if len(got) != 2 {
		t.Fatalf("expected 2 warnings after filtering, got %d: %v", len(got), got)
	}
	for _, w := range got {
		if w.Uses != "" {
			t.Errorf("unexpected pin warning survived filter: %+v", w)
		}
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

	warnings, err := CheckFile(f, nil, DisabledChecks{})
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

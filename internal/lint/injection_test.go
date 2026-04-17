package lint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDangerousExpressionsIn(t *testing.T) {
	tests := []struct {
		line    string
		wantLen int
	}{
		{`echo "${{ github.event.issue.title }}"`, 1},
		{`echo "${{ github.event.pull_request.body }}"`, 1},
		{`echo "${{ github.head_ref }}"`, 1},
		{`echo "${{ github.event.commits[0].message }}"`, 1},
		// Multiple dangerous expressions on one line
		{`echo "${{ github.event.issue.title }} ${{ github.event.comment.body }}"`, 2},
		// Safe — SHA context is not user-controlled
		{`echo "${{ github.sha }}"`, 0},
		{`echo "${{ github.repository }}"`, 0},
		{`echo "${{ secrets.TOKEN }}"`, 0},
		// Safe — env var indirection
		{`echo "$TITLE"`, 0},
		// Case-insensitive match
		{`echo "${{ GITHUB.EVENT.ISSUE.TITLE }}"`, 1},
	}

	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			got := dangerousExpressionsIn(tt.line)
			if len(got) != tt.wantLen {
				t.Errorf("dangerousExpressionsIn(%q) = %v (len %d), want len %d", tt.line, got, len(got), tt.wantLen)
			}
		})
	}
}

func TestCheckRunInjectionInlineScalar(t *testing.T) {
	// Inline scalar: run: echo "${{ expr }}" — content is ON the run: line
	ws := checkRunInjection(`echo "${{ github.event.issue.title }}"`, "ci.yml", 10, 0)
	if len(ws) != 1 {
		t.Fatalf("expected 1 warning, got %d", len(ws))
	}
	if ws[0].Line != 10 {
		t.Errorf("expected line 10, got %d", ws[0].Line)
	}
	if !strings.Contains(ws[0].Message, "script injection") {
		t.Errorf("unexpected message: %s", ws[0].Message)
	}
}

func TestCheckRunInjectionBlockScalar(t *testing.T) {
	// Block scalar (run: |): content starts on baseLine+1.
	// Line 0 of content = baseLine+1, line 1 = baseLine+2, etc.
	runValue := "safe line\necho \"${{ github.event.issue.title }}\"\nanother safe line"
	ws := checkRunInjection(runValue, "ci.yml", 5, 1)
	if len(ws) != 1 {
		t.Fatalf("expected 1 warning, got %d", len(ws))
	}
	// Dangerous expr is on content line 1 (0-indexed), so actual line = 5 + 1 + 1 = 7
	if ws[0].Line != 7 {
		t.Errorf("expected line 7, got %d", ws[0].Line)
	}
}

func TestCheckFileWithInjection(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "ci.yml")
	content := `
name: CI
on: [issues]
jobs:
  triage:
    runs-on: ubuntu-latest
    steps:
      - name: Echo title
        run: echo "${{ github.event.issue.title }}"
      - name: Safe step
        env:
          TITLE: ${{ github.event.issue.title }}
        run: echo "$TITLE"
`
	if err := os.WriteFile(f, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	warnings, err := CheckFile(f, nil, DisabledChecks{})
	if err != nil {
		t.Fatal(err)
	}

	var injectionWarnings []Warning
	for _, w := range warnings {
		if strings.Contains(w.Message, "script injection") {
			injectionWarnings = append(injectionWarnings, w)
		}
	}

	if len(injectionWarnings) != 1 {
		t.Fatalf("expected 1 injection warning, got %d: %v", len(injectionWarnings), injectionWarnings)
	}
	if injectionWarnings[0].Line != 9 {
		t.Errorf("expected line 9, got %d", injectionWarnings[0].Line)
	}
}

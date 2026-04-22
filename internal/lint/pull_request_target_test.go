package lint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func parseYAMLDoc(t *testing.T, content string) *yaml.Node {
	t.Helper()
	var root yaml.Node
	if err := yaml.Unmarshal([]byte(content), &root); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}
	return &root
}

// --- hasPullRequestTarget ---

func TestHasPullRequestTarget_Scalar(t *testing.T) {
	doc := parseYAMLDoc(t, "on: pull_request_target\njobs: {}\n")
	if !hasPullRequestTarget(doc.Content[0]) {
		t.Error("expected true for scalar pull_request_target")
	}
}

func TestHasPullRequestTarget_Sequence(t *testing.T) {
	doc := parseYAMLDoc(t, "on: [push, pull_request_target]\njobs: {}\n")
	if !hasPullRequestTarget(doc.Content[0]) {
		t.Error("expected true for pull_request_target in sequence")
	}
}

func TestHasPullRequestTarget_Mapping(t *testing.T) {
	content := "on:\n  pull_request_target:\n    types: [opened]\njobs: {}\n"
	doc := parseYAMLDoc(t, content)
	if !hasPullRequestTarget(doc.Content[0]) {
		t.Error("expected true for pull_request_target in mapping")
	}
}

func TestHasPullRequestTarget_OtherTrigger(t *testing.T) {
	doc := parseYAMLDoc(t, "on: pull_request\njobs: {}\n")
	if hasPullRequestTarget(doc.Content[0]) {
		t.Error("expected false for pull_request trigger")
	}
}

func TestHasPullRequestTarget_SequenceWithoutPRT(t *testing.T) {
	doc := parseYAMLDoc(t, "on: [push, pull_request]\njobs: {}\n")
	if hasPullRequestTarget(doc.Content[0]) {
		t.Error("expected false when pull_request_target not in sequence")
	}
}

// --- checkPullRequestTarget ---

const prtCheckoutHeadRef = `
name: CI
on: pull_request_target
permissions: {}
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@11bd317f7bc71dd3eee3f1bf1c58bc03de17e433
        with:
          ref: ${{ github.event.pull_request.head.ref }}
`

const prtCheckoutHeadSHA = `
name: CI
on: pull_request_target
permissions: {}
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@11bd317f7bc71dd3eee3f1bf1c58bc03de17e433
        with:
          ref: ${{ github.event.pull_request.head.sha }}
`

const prtCheckoutHeadRefShorthand = `
name: CI
on: pull_request_target
permissions: {}
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@11bd317f7bc71dd3eee3f1bf1c58bc03de17e433
        with:
          ref: ${{ github.head_ref }}
`

const prtCheckoutEmbeddedRef = `
name: CI
on: pull_request_target
permissions: {}
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@11bd317f7bc71dd3eee3f1bf1c58bc03de17e433
        with:
          ref: refs/heads/${{ github.head_ref }}
`

const prtCheckoutForkRepo = `
name: CI
on: pull_request_target
permissions: {}
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@11bd317f7bc71dd3eee3f1bf1c58bc03de17e433
        with:
          repository: ${{ github.event.pull_request.head.repo.full_name }}
`

const prtCheckoutNoRef = `
name: CI
on: pull_request_target
permissions: {}
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@11bd317f7bc71dd3eee3f1bf1c58bc03de17e433
`

const prtCheckoutBaseRef = `
name: CI
on: pull_request_target
permissions: {}
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@11bd317f7bc71dd3eee3f1bf1c58bc03de17e433
        with:
          ref: ${{ github.sha }}
`

const prCheckoutHeadRef = `
name: CI
on: pull_request
permissions: {}
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@11bd317f7bc71dd3eee3f1bf1c58bc03de17e433
        with:
          ref: ${{ github.event.pull_request.head.ref }}
`

func TestCheckPullRequestTarget_DangerousRef(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantMsg string
	}{
		{"head.ref", prtCheckoutHeadRef, "PR head ref"},
		{"head.sha", prtCheckoutHeadSHA, "PR head ref"},
		{"github.head_ref", prtCheckoutHeadRefShorthand, "PR head ref"},
		{"embedded ref", prtCheckoutEmbeddedRef, "PR head ref"},
		{"fork repository", prtCheckoutForkRepo, "PR fork repository"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := parseYAMLDoc(t, tt.content)
			ws := checkPullRequestTarget(root, "ci.yml")
			if len(ws) != 1 {
				t.Fatalf("expected 1 warning, got %d: %v", len(ws), ws)
			}
			if !strings.Contains(ws[0].Message, tt.wantMsg) {
				t.Errorf("expected message to contain %q, got: %s", tt.wantMsg, ws[0].Message)
			}
			if !strings.Contains(ws[0].Message, "pull_request_target") {
				t.Errorf("expected message to mention pull_request_target, got: %s", ws[0].Message)
			}
		})
	}
}

func TestCheckPullRequestTarget_Safe(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{"no ref in checkout", prtCheckoutNoRef},
		{"safe base ref", prtCheckoutBaseRef},
		{"pull_request trigger not prt", prCheckoutHeadRef},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := parseYAMLDoc(t, tt.content)
			ws := checkPullRequestTarget(root, "ci.yml")
			// Filter out any non-PRT warnings
			var prtWarnings []Warning
			for _, w := range ws {
				if strings.Contains(w.Message, "pull_request_target") {
					prtWarnings = append(prtWarnings, w)
				}
			}
			if len(prtWarnings) != 0 {
				t.Errorf("expected no PRT warnings, got: %v", prtWarnings)
			}
		})
	}
}

// --- Integration via CheckFile ---

func TestCheckFile_PullRequestTarget(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "ci.yml")
	if err := os.WriteFile(f, []byte(prtCheckoutHeadRef), 0o644); err != nil {
		t.Fatal(err)
	}

	warnings, err := CheckFile(f, nil, DisabledChecks{})
	if err != nil {
		t.Fatal(err)
	}

	var prtWarnings []Warning
	for _, w := range warnings {
		if strings.Contains(w.Message, "pull_request_target") {
			prtWarnings = append(prtWarnings, w)
		}
	}

	if len(prtWarnings) != 1 {
		t.Fatalf("expected 1 PRT warning via CheckFile, got %d: %v", len(prtWarnings), prtWarnings)
	}
	if !strings.Contains(prtWarnings[0].Message, "PR head ref") {
		t.Errorf("unexpected message: %s", prtWarnings[0].Message)
	}
}

func TestCheckFile_PullRequestTarget_Disabled(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "ci.yml")
	if err := os.WriteFile(f, []byte(prtCheckoutHeadRef), 0o644); err != nil {
		t.Fatal(err)
	}

	warnings, err := CheckFile(f, nil, DisabledChecks{PullRequestTarget: true})
	if err != nil {
		t.Fatal(err)
	}

	for _, w := range warnings {
		if strings.Contains(w.Message, "pull_request_target") {
			t.Errorf("expected PRT check to be disabled, got: %s", w.Message)
		}
	}
}

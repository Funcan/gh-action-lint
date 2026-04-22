package lint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFixLineAlreadyPinned(t *testing.T) {
	line := "      - uses: actions/checkout@11bd317f7bc71dd3eee3f1bf1c58bc03de17e433"
	got, from, _, err := fixLine(line, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got != line {
		t.Errorf("expected no change for already-pinned line, got: %s", got)
	}
	if from != "" {
		t.Errorf("expected empty from for already-pinned line")
	}
}

func TestFixLineLocal(t *testing.T) {
	line := "      - uses: ./local-action"
	got, _, _, err := fixLine(line, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got != line {
		t.Errorf("expected no change for local action")
	}
}

func TestFixLineIgnored(t *testing.T) {
	il := &IgnoreList{patterns: map[string]bool{"actions/checkout": true}}
	line := "      - uses: actions/checkout@v4"
	got, _, _, err := fixLine(line, il, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got != line {
		t.Errorf("expected no change for ignored action")
	}
}

func TestFixLineNoMatch(t *testing.T) {
	line := "      run: echo hello"
	got, _, _, err := fixLine(line, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got != line {
		t.Errorf("expected no change for non-uses line")
	}
}

func TestFixLinePreservesIndentation(t *testing.T) {
	// Use a mock resolver that returns a fixed SHA.
	resolver := &Resolver{
		token: "",
		cache: map[string]string{"actions/checkout@v4": "aabbccddee" + "aabbccddee" + "aabbccddee" + "aabbccdd"},
	}
	line := "        - uses: actions/checkout@v4"
	got, from, to, err := fixLine(line, nil, resolver)
	if err != nil {
		t.Fatal(err)
	}
	if from != "actions/checkout@v4" {
		t.Errorf("unexpected from: %s", from)
	}
	wantSHA := "aabbccddeeaabbccddeeaabbccddeeaabbccdd"
	if !strings.Contains(got, wantSHA) {
		t.Errorf("expected SHA in output, got: %s", got)
	}
	if !strings.Contains(got, "# v4") {
		t.Errorf("expected original ref as comment, got: %s", got)
	}
	if got[:9] != "        -" {
		t.Errorf("expected indentation preserved, got: %q", got)
	}
	_ = to
}

func TestFixLineStripsExistingComment(t *testing.T) {
	resolver := &Resolver{
		token: "",
		cache: map[string]string{"actions/cache@v3": "aabbccddee" + "aabbccddee" + "aabbccddee" + "aabbccdd"},
	}
	line := "      - uses: actions/cache@v3  # old comment"
	got, _, _, err := fixLine(line, nil, resolver)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "old comment") {
		t.Errorf("expected old comment to be replaced, got: %s", got)
	}
	if !strings.Contains(got, "# v3") {
		t.Errorf("expected '# v3' in output, got: %s", got)
	}
}

func TestFixFile_DisabledPins(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "ci.yml")
	content := "name: CI\non: push\njobs:\n  build:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: actions/checkout@v4\n"
	if err := os.WriteFile(f, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	// pins disabled — only the permissions fix should run (resolver not needed)
	results, err := FixFile(f, nil, nil, DisabledChecks{Pins: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].To != "permissions: contents: read" {
		t.Errorf("expected only permissions fix, got: %v", results)
	}
	data, _ := os.ReadFile(f)
	if !strings.Contains(string(data), "actions/checkout@v4") {
		t.Error("expected checkout to remain unpinned when pins check is disabled")
	}
	if !strings.Contains(string(data), "contents: read") {
		t.Error("expected contents: read to be added")
	}
}

func TestFixFile_DisabledPermissions(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "ci.yml")
	content := "name: CI\non: push\njobs:\n  build:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: actions/checkout@v4\n"
	if err := os.WriteFile(f, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	resolver := &Resolver{
		cache: map[string]string{"actions/checkout@v4": "aabbccddeeaabbccddeeaabbccddeeaabbccdd"},
	}
	// permissions disabled — only the pin fix should run
	results, err := FixFile(f, nil, resolver, DisabledChecks{Permissions: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].From != "actions/checkout@v4" {
		t.Errorf("expected only pins fix, got: %v", results)
	}
	data, _ := os.ReadFile(f)
	if strings.Contains(string(data), "permissions") {
		t.Error("expected permissions block NOT to be added when permissions check is disabled")
	}
}

func TestFixPermissionsMissing(t *testing.T) {
	data := []byte("name: CI\non: push\njobs:\n  build:\n    runs-on: ubuntu-latest\n")
	lines := strings.Split(string(data), "\n")
	newLines, r := fixPermissions(lines, data)
	if r == nil {
		t.Fatal("expected a FixResult for workflow missing permissions")
	}
	if r.To != "permissions: contents: read" {
		t.Errorf("unexpected To: %s", r.To)
	}
	joined := strings.Join(newLines, "\n")
	if !strings.Contains(joined, "contents: read") {
		t.Errorf("expected contents: read in output:\n%s", joined)
	}
	// permissions: and contents: read must appear before jobs:
	permIdx := strings.Index(joined, "permissions:")
	jobsIdx := strings.Index(joined, "jobs:")
	if permIdx > jobsIdx {
		t.Errorf("permissions block should appear before jobs:")
	}
}

func TestFixPermissionsAlreadyPresent(t *testing.T) {
	data := []byte("name: CI\non: push\npermissions: {}\njobs:\n  build:\n    runs-on: ubuntu-latest\n")
	lines := strings.Split(string(data), "\n")
	_, r := fixPermissions(lines, data)
	if r != nil {
		t.Errorf("expected no fix when permissions already declared, got: %+v", r)
	}
}

func TestFixPermissionsCompositeAction(t *testing.T) {
	data := []byte("name: My Action\nruns:\n  using: composite\n  steps: []\n")
	lines := strings.Split(string(data), "\n")
	_, r := fixPermissions(lines, data)
	if r != nil {
		t.Errorf("expected no fix for composite action (no jobs: key), got: %+v", r)
	}
}

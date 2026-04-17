package lint

import (
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
	if !containsSubstring(got, wantSHA) {
		t.Errorf("expected SHA in output, got: %s", got)
	}
	if !containsSubstring(got, "# v4") {
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
	if containsSubstring(got, "old comment") {
		t.Errorf("expected old comment to be replaced, got: %s", got)
	}
	if !containsSubstring(got, "# v3") {
		t.Errorf("expected '# v3' in output, got: %s", got)
	}
}

func TestFixPermissionsMissing(t *testing.T) {
	data := []byte("name: CI\non: push\njobs:\n  build:\n    runs-on: ubuntu-latest\n")
	lines := strings.Split(string(data), "\n")
	newLines, r := fixPermissions(lines, data)
	if r == nil {
		t.Fatal("expected a FixResult for workflow missing permissions")
	}
	if r.To != "permissions: {}" {
		t.Errorf("unexpected To: %s", r.To)
	}
	joined := strings.Join(newLines, "\n")
	if !containsSubstring(joined, "permissions: {}") {
		t.Errorf("expected permissions: {} in output:\n%s", joined)
	}
	// permissions: {} must appear before jobs:
	permIdx := strings.Index(joined, "permissions: {}")
	jobsIdx := strings.Index(joined, "jobs:")
	if permIdx > jobsIdx {
		t.Errorf("permissions: {} should appear before jobs:")
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

func containsSubstring(s, sub string) bool {	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		}())
}

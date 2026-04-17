package lint

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func parseYAMLRoot(t *testing.T, content string) *yaml.Node {
	t.Helper()
	var root yaml.Node
	if err := yaml.Unmarshal([]byte(content), &root); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}
	return &root
}

func TestCheckPermissions_Missing(t *testing.T) {
	content := `
name: CI
on: push
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - run: echo hello
`
	root := parseYAMLRoot(t, content)
	ws := checkPermissions(root, "ci.yml")
	if len(ws) != 1 {
		t.Fatalf("expected 1 warning, got %d: %v", len(ws), ws)
	}
	if !strings.Contains(ws[0].Message, "no top-level permissions") {
		t.Errorf("unexpected message: %s", ws[0].Message)
	}
}

func TestCheckPermissions_Explicit(t *testing.T) {
	content := `
name: CI
on: push
permissions:
  contents: read
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - run: echo hello
`
	root := parseYAMLRoot(t, content)
	ws := checkPermissions(root, "ci.yml")
	if len(ws) != 0 {
		t.Errorf("expected no warnings for explicit permissions, got: %v", ws)
	}
}

func TestCheckPermissions_Empty(t *testing.T) {
	// permissions: {} means no access at all — safest setting
	content := `
name: CI
on: push
permissions: {}
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - run: echo hello
`
	root := parseYAMLRoot(t, content)
	ws := checkPermissions(root, "ci.yml")
	if len(ws) != 0 {
		t.Errorf("expected no warnings for empty permissions, got: %v", ws)
	}
}

func TestCheckPermissions_WriteAll(t *testing.T) {
	content := `
name: CI
on: push
permissions: write-all
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - run: echo hello
`
	root := parseYAMLRoot(t, content)
	ws := checkPermissions(root, "ci.yml")
	if len(ws) != 1 {
		t.Fatalf("expected 1 warning, got %d: %v", len(ws), ws)
	}
	if !strings.Contains(ws[0].Message, "write-all") {
		t.Errorf("unexpected message: %s", ws[0].Message)
	}
}

func TestCheckPermissions_JobWriteAll(t *testing.T) {
	content := `
name: CI
on: push
permissions:
  contents: read
jobs:
  build:
    runs-on: ubuntu-latest
    permissions: write-all
    steps:
      - run: echo hello
`
	root := parseYAMLRoot(t, content)
	ws := checkPermissions(root, "ci.yml")
	if len(ws) != 1 {
		t.Fatalf("expected 1 warning for job write-all, got %d: %v", len(ws), ws)
	}
	if !strings.Contains(ws[0].Message, `"build"`) {
		t.Errorf("expected job name in message, got: %s", ws[0].Message)
	}
}

func TestCheckPermissions_CompositeActionSkipped(t *testing.T) {
	// Composite actions have runs: not jobs: — should not be checked
	content := `
name: My Action
runs:
  using: composite
  steps:
    - run: echo hello
      shell: bash
`
	root := parseYAMLRoot(t, content)
	ws := checkPermissions(root, "action.yml")
	if len(ws) != 0 {
		t.Errorf("expected no warnings for composite action, got: %v", ws)
	}
}

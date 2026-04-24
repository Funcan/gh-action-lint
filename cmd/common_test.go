package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// initTestRepo creates a temporary git repository, writes the given files
// (relative paths → content), and stages the files listed in toStage.
// It returns the repo root.
func initTestRepo(t *testing.T, files map[string]string, toStage []string) string {
	t.Helper()
	dir := t.TempDir()

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("command %v failed: %v\n%s", args, err, out)
		}
	}

	run("git", "init")
	run("git", "config", "user.email", "test@example.com")
	run("git", "config", "user.name", "Test")

	for rel, content := range files {
		abs := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	for _, rel := range toStage {
		run("git", "add", rel)
	}

	return dir
}

const minimalWorkflow = `name: CI
on: push
permissions: {}
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@11bd317f7bc71dd3eee3f1bf1c58bc03de17e433
`

func TestFilterStagedFiles_OnlyStagedReturned(t *testing.T) {
	files := map[string]string{
		".github/workflows/ci.yml":   minimalWorkflow,
		".github/workflows/cd.yml":   minimalWorkflow,
		".github/workflows/lint.yml": minimalWorkflow,
	}
	repoRoot := initTestRepo(t, files, []string{
		".github/workflows/ci.yml",
		".github/workflows/cd.yml",
		// lint.yml is NOT staged
	})
	chdir(t, repoRoot)

	candidates := []string{
		filepath.Join(repoRoot, ".github/workflows/ci.yml"),
		filepath.Join(repoRoot, ".github/workflows/cd.yml"),
		filepath.Join(repoRoot, ".github/workflows/lint.yml"),
	}

	got, err := filterStagedFiles(repoRoot, candidates)
	if err != nil {
		t.Fatal(err)
	}

	if len(got) != 2 {
		t.Fatalf("expected 2 staged files, got %d: %v", len(got), got)
	}
	for _, f := range got {
		if filepath.Base(f) == "lint.yml" {
			t.Errorf("unstaged file lint.yml should not have been returned")
		}
	}
}

func TestFilterStagedFiles_NoneStaged(t *testing.T) {
	files := map[string]string{
		".github/workflows/ci.yml": minimalWorkflow,
	}
	repoRoot := initTestRepo(t, files, nil) // nothing staged
	chdir(t, repoRoot)

	candidates := []string{
		filepath.Join(repoRoot, ".github/workflows/ci.yml"),
	}

	got, err := filterStagedFiles(repoRoot, candidates)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("expected no staged files, got %d: %v", len(got), got)
	}
}

func TestFilterStagedFiles_AllStaged(t *testing.T) {
	files := map[string]string{
		".github/workflows/ci.yml": minimalWorkflow,
		".github/workflows/cd.yml": minimalWorkflow,
	}
	repoRoot := initTestRepo(t, files, []string{
		".github/workflows/ci.yml",
		".github/workflows/cd.yml",
	})
	chdir(t, repoRoot)

	candidates := []string{
		filepath.Join(repoRoot, ".github/workflows/ci.yml"),
		filepath.Join(repoRoot, ".github/workflows/cd.yml"),
	}

	got, err := filterStagedFiles(repoRoot, candidates)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 staged files, got %d", len(got))
	}
}

func TestFilterStagedFiles_StagedButNotActionFile(t *testing.T) {
	// A non-action file is staged; the candidates list (action files) is empty.
	files := map[string]string{
		"README.md": "hello",
	}
	repoRoot := initTestRepo(t, files, []string{"README.md"})
	chdir(t, repoRoot)

	got, err := filterStagedFiles(repoRoot, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("expected no action files, got %d: %v", len(got), got)
	}
}

func TestGitOutput_IncludesStderr(t *testing.T) {
	// Run git rev-parse outside any git repo so git writes "fatal: ..." to stderr.
	notARepo := t.TempDir()
	chdir(t, notARepo)

	_, err := gitOutput("rev-parse", "--show-toplevel")
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "fatal") {
		t.Errorf("expected git's fatal error message in error, got: %q", msg)
	}
}

// chdir changes the working directory for the duration of the test,
// restoring it on cleanup.
func chdir(t *testing.T, dir string) {
	t.Helper()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(orig); err != nil {
			t.Errorf("failed to restore working directory: %v", err)
		}
	})
}

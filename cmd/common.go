package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/funcan/gh-action-lint/internal/lint"
)

// runContext holds the common state prepared before any subcommand runs.
type runContext struct {
	disabled lint.DisabledChecks
	repoRoot string
	ignore   *lint.IgnoreList
	files    []string
}

// relPath returns the path relative to the repo root.
func (rc *runContext) relPath(absPath string) string {
	return strings.TrimPrefix(absPath, rc.repoRoot+"/")
}

// prepareRun parses flags, locates the repo, loads the ignore list, and
// discovers action files. It returns nil (with no error) when there are no
// files to process; the "no files found" message is printed before returning.
func prepareRun(disableFlag string) (*runContext, error) {
	disabled, err := lint.ParseDisabledChecks(disableFlag)
	if err != nil {
		return nil, err
	}

	repoRoot, err := gitRepoRoot()
	if err != nil {
		return nil, fmt.Errorf("not inside a git repository: %w", err)
	}

	ignore, err := lint.LoadIgnoreFile(repoRoot)
	if err != nil {
		return nil, fmt.Errorf("loading ignore file: %w", err)
	}

	files, err := lint.FindActionFiles(repoRoot)
	if err != nil {
		return nil, err
	}

	if len(files) == 0 {
		fmt.Fprintln(os.Stderr, "no GitHub Actions files found")
		return nil, nil
	}

	return &runContext{
		disabled: disabled,
		repoRoot: repoRoot,
		ignore:   ignore,
		files:    files,
	}, nil
}

func gitRepoRoot() (string, error) {
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

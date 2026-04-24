package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
// If staged is true, only files with staged changes are checked.
func prepareRun(disableFlag string, staged bool) (*runContext, error) {
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

	if staged {
		files, err = filterStagedFiles(repoRoot, files)
		if err != nil {
			return nil, fmt.Errorf("getting staged files: %w", err)
		}
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

// filterStagedFiles returns only those files from candidates that are currently
// staged in git (added, copied, modified, or renamed).
func filterStagedFiles(repoRoot string, candidates []string) ([]string, error) {
	out, err := gitOutput("diff", "--cached", "--name-only", "--diff-filter=ACMR")
	if err != nil {
		return nil, err
	}

	staged := make(map[string]bool)
	for _, rel := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if rel != "" {
			staged[filepath.Join(repoRoot, rel)] = true
		}
	}

	var filtered []string
	for _, f := range candidates {
		if staged[f] {
			filtered = append(filtered, f)
		}
	}
	return filtered, nil
}

func gitRepoRoot() (string, error) {
	out, err := gitOutput("rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// gitOutput runs a git subcommand and returns its stdout. If the command fails,
// git's stderr is included in the returned error so the user sees the message.
func gitOutput(args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && len(exitErr.Stderr) > 0 {
			return nil, fmt.Errorf("%w: %s", err, strings.TrimSpace(string(exitErr.Stderr)))
		}
		return nil, err
	}
	return out, nil
}

package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/funcan/gh-action-lint/internal/lint"
	"github.com/spf13/cobra"
)

var checkCmd = &cobra.Command{
	Use:   "check",
	Short: "Check for actions pinned to a tag rather than a SHA",
	RunE:  runCheck,
}

func runCheck(cmd *cobra.Command, args []string) error {
	repoRoot, err := gitRepoRoot()
	if err != nil {
		return fmt.Errorf("not inside a git repository: %w", err)
	}

	files, err := lint.FindActionFiles(repoRoot)
	if err != nil {
		return err
	}

	if len(files) == 0 {
		fmt.Fprintln(os.Stderr, "no GitHub Actions files found")
		return nil
	}

	var total int
	for _, f := range files {
		warnings, err := lint.CheckFile(f)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: skipping %s: %v\n", f, err)
			continue
		}
		for _, w := range warnings {
			// Print path relative to repo root for readability
			rel := strings.TrimPrefix(f, repoRoot+"/")
			fmt.Printf("%s:%d: action not pinned to a SHA: %s\n", rel, w.Line, w.Uses)
			total++
		}
	}

	if total > 0 {
		os.Exit(1)
	}
	return nil
}

func gitRepoRoot() (string, error) {
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

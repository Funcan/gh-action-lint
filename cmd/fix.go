package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/funcan/gh-action-lint/internal/lint"
	"github.com/spf13/cobra"
)

var fixCmd = &cobra.Command{
	Use:   "fix",
	Short: "Pin unpinned actions to their resolved commit SHA",
	Long: `Pin all unpinned action refs in the repository's workflow and composite action
files to their resolved commit SHA, adding the original ref as a comment.

For example:
  uses: actions/checkout@v4
becomes:
  uses: actions/checkout@11bd317f7bc71dd3eee3f1bf1c58bc03de17e433 # v4

Actions listed in .gh-lint-ignore are not modified.
Set GITHUB_TOKEN to authenticate requests and avoid rate limits.`,
	RunE: runFix,
}

func init() {
	rootCmd.AddCommand(fixCmd)
}

func runFix(cmd *cobra.Command, args []string) error {
	repoRoot, err := gitRepoRoot()
	if err != nil {
		return fmt.Errorf("not inside a git repository: %w", err)
	}

	ignore, err := lint.LoadIgnoreFile(repoRoot)
	if err != nil {
		return fmt.Errorf("loading ignore file: %w", err)
	}

	files, err := lint.FindActionFiles(repoRoot)
	if err != nil {
		return err
	}

	if len(files) == 0 {
		fmt.Fprintln(os.Stderr, "no GitHub Actions files found")
		return nil
	}

	token := os.Getenv("GITHUB_TOKEN")
	resolver := lint.NewResolver(token)

	var totalFixed int
	for _, f := range files {
		results, err := lint.FixFile(f, ignore, resolver)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %s: %v\n", f, err)
			continue
		}

		rel := strings.TrimPrefix(f, repoRoot+"/")
		for _, r := range results {
			if r.Err != nil {
				fmt.Fprintf(os.Stderr, "warning: %s:%d: cannot pin %s: %v\n", rel, r.Line, r.From, r.Err)
			} else {
				fmt.Printf("%s:%d: %s -> %s\n", rel, r.Line, r.From, r.To)
				totalFixed++
			}
		}
	}

	if totalFixed == 0 && len(files) > 0 {
		fmt.Println("nothing to fix")
	}

	return nil
}

package cmd

import (
	"fmt"
	"os"

	"github.com/funcan/gh-action-lint/internal/lint"
	"github.com/spf13/cobra"
)

var fixDisable string

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
	fixCmd.Flags().StringVar(&fixDisable, "disable-check", "", "comma-separated list of fixes to skip (pins,permissions)")
}

func runFix(cmd *cobra.Command, args []string) error {
	ctx, err := prepareRun(fixDisable)
	if err != nil {
		return err
	}
	if ctx == nil {
		return nil
	}

	token := os.Getenv("GITHUB_TOKEN")
	resolver := lint.NewResolver(token)

	var totalFixed int
	for _, f := range ctx.files {
		results, err := lint.FixFile(f, ctx.ignore, resolver, ctx.disabled)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %s: %v\n", f, err)
			continue
		}

		rel := ctx.relPath(f)
		for _, r := range results {
			if r.Err != nil {
				fmt.Fprintf(os.Stderr, "warning: %s:%d: cannot pin %s: %v\n", rel, r.Line, r.From, r.Err)
			} else {
				fmt.Printf("%s:%d: %s -> %s\n", rel, r.Line, r.From, r.To)
				totalFixed++
			}
		}
	}

	if totalFixed == 0 && len(ctx.files) > 0 {
		fmt.Println("nothing to fix")
	}

	return nil
}

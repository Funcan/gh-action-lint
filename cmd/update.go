package cmd

import (
	"fmt"
	"os"

	"github.com/funcan/gh-action-lint/internal/lint"
	"github.com/spf13/cobra"
)

var updateNoMajorBump bool
var updateAction string

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update actions to their latest upstream release",
	Long: `Update every action in the repository's workflow and composite action files to
its latest upstream release tag, pinned to that tag's commit SHA:

  uses: actions/checkout@v4
becomes:
  uses: actions/checkout@<sha> # v5

For already-pinned actions the current version is read from the pin comment.
The tag's precision is preserved where possible (v4 -> v5, v4.1.0 -> v5.0.1).
Refs that are not release versions (e.g. branches) and actions listed in
.gh-lint-ignore are not modified.

With --no-major-bump, actions stay on their current major version and a warning
is printed if a newer major version is available.

The GitHub token is taken from GH_TOKEN, GITHUB_TOKEN, or 'gh auth token'.`,
	RunE: runUpdate,
}

func init() {
	rootCmd.AddCommand(updateCmd)
	updateCmd.Flags().BoolVar(&updateNoMajorBump, "no-major-bump", false, "only update within the current major version; warn if a newer major exists")
	updateCmd.Flags().StringVar(&updateAction, "action", "", "only update the named action (e.g. actions/checkout)")
}

func runUpdate(cmd *cobra.Command, args []string) error {
	ctx, err := prepareRun("", false)
	if err != nil {
		return err
	}
	if ctx == nil {
		return nil
	}

	resolver := lint.NewResolver(githubToken())
	opts := lint.UpdateOptions{NoMajorBump: updateNoMajorBump, Action: updateAction}

	var totalUpdated int
	var anyErrors bool
	for _, f := range ctx.files {
		results, err := lint.UpdateFile(f, ctx.ignore, resolver, opts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %s: %v\n", f, err)
			anyErrors = true
			continue
		}

		rel := ctx.relPath(f)
		for _, r := range results {
			if r.Err != nil {
				fmt.Fprintf(os.Stderr, "error: %s:%d: cannot update %s: %v\n", rel, r.Line, r.From, r.Err)
				anyErrors = true
				continue
			}
			if r.To != "" {
				fmt.Printf("%s:%d: %s -> %s\n", rel, r.Line, r.From, r.To)
				totalUpdated++
			}
			if r.Warning != "" {
				fmt.Printf("%s:%d: warning: %s\n", rel, r.Line, r.Warning)
			}
		}
	}

	if !anyErrors && totalUpdated == 0 {
		fmt.Println("everything is up to date")
	}

	if anyErrors {
		os.Exit(1)
	}
	return nil
}

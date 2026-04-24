package cmd

import (
	"fmt"
	"os"

	"github.com/funcan/gh-action-lint/internal/lint"
	"github.com/spf13/cobra"
)

var recursive bool
var checkDisable string
var checkStaged bool

var checkCmd = &cobra.Command{
	Use:   "check",
	Short: "Check for actions pinned to a tag rather than a SHA",
	Long: `Check all GitHub Actions workflows and composite actions in the current git
repository for any 'uses:' references not pinned to a full commit SHA.

With --recursive, also fetches each used action from GitHub and checks whether
it in turn uses any unpinned actions, traversing the full dependency graph.
Set GITHUB_TOKEN to authenticate requests and avoid rate limits.`,
	RunE: runCheck,
}

func init() {
	checkCmd.Flags().BoolVarP(&recursive, "recursive", "r", false, "also check actions used by the repo's actions")
	checkCmd.Flags().StringVar(&checkDisable, "disable-check", "", "comma-separated list of checks to skip (pins,injections,permissions)")
	checkCmd.Flags().BoolVar(&checkStaged, "staged", false, "only check files that are staged in git")
}

func runCheck(cmd *cobra.Command, args []string) error {
	ctx, err := prepareRun(checkDisable, checkStaged)
	if err != nil {
		return err
	}
	if ctx == nil {
		return nil
	}

	var total int
	var allExternalUses []string

	for _, f := range ctx.files {
		warnings, err := lint.CheckFile(f, ctx.ignore, ctx.disabled)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: skipping %s: %v\n", f, err)
			continue
		}
		for _, w := range warnings {
			fmt.Printf("%s:%d: %s\n", ctx.relPath(f), w.Line, w.Message)
			total++
		}

		if recursive {
			uses, err := lint.ExternalUsesFromFile(f)
			if err == nil {
				allExternalUses = append(allExternalUses, uses...)
			}
		}
	}

	if recursive {
		token := os.Getenv("GITHUB_TOKEN")
		fmt.Fprintf(os.Stderr, "checking %d external action(s) recursively...\n", len(dedupe(allExternalUses)))
		remoteWarnings, err := lint.CheckRecursive(dedupe(allExternalUses), token, ctx.ignore, ctx.disabled)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: recursive check failed: %v\n", err)
		}
		for _, w := range remoteWarnings {
			fmt.Printf("%s:%d: %s\n", w.File, w.Line, w.Message)
			total++
		}
	}

	if total > 0 {
		os.Exit(1)
	}
	return nil
}

func dedupe(ss []string) []string {
	seen := make(map[string]bool, len(ss))
	out := make([]string, 0, len(ss))
	for _, s := range ss {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

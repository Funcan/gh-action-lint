package lint

import (
	"fmt"
	"regexp"
	"strings"
)

// exprRe matches a GitHub Actions expression: ${{ ... }}
// Expressions don't contain }} internally, so [^}]+ is sufficient.
var exprRe = regexp.MustCompile(`\$\{\{[^}]+\}\}`)

// dangerousContexts lists expression substrings that reference user-controlled
// input. Any of these appearing directly inside a run: step is a script
// injection risk because an attacker can embed shell metacharacters.
var dangerousContexts = []string{
	"github.event.issue.title",
	"github.event.issue.body",
	"github.event.pull_request.title",
	"github.event.pull_request.body",
	"github.event.pull_request.head.ref",
	"github.event.pull_request.head.label",
	"github.event.discussion.title",
	"github.event.discussion.body",
	"github.event.comment.body",
	"github.event.review.body",
	"github.event.review_comment.body",
	"github.event.commits",
	"github.event.head_commit.message",
	"github.event.head_commit.author.name",
	"github.event.head_commit.author.email",
	"github.event.pages",
	"github.head_ref",
}

// checkRunInjection scans a run: value for dangerous expression interpolation.
// baseLine is the line of the run: key; lineOffset is 1 for block scalars
// (where content starts on the next line) and 0 for inline scalars.
func checkRunInjection(runValue, file string, baseLine, lineOffset int) []Warning {
	var warnings []Warning
	for i, line := range strings.Split(runValue, "\n") {
		for _, expr := range dangerousExpressionsIn(line) {
			warnings = append(warnings, Warning{
				File:    file,
				Line:    baseLine + lineOffset + i,
				Message: fmt.Sprintf("script injection: %s used directly in run step", expr),
			})
		}
	}
	return warnings
}

// dangerousExpressionsIn returns every ${{ }} expression on a single line that
// references a user-controlled context.
func dangerousExpressionsIn(line string) []string {
	var found []string
	for _, expr := range exprRe.FindAllString(line, -1) {
		lower := strings.ToLower(expr)
		for _, ctx := range dangerousContexts {
			if strings.Contains(lower, ctx) {
				found = append(found, expr)
				break
			}
		}
	}
	return found
}

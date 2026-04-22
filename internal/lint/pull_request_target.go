package lint

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// prHeadRefContexts lists expression substrings that reference the PR's head
// ref or SHA. Using any of these as the ref: in an actions/checkout step
// within a pull_request_target workflow checks out attacker-controlled code.
var prHeadRefContexts = []string{
	"github.event.pull_request.head.ref",
	"github.event.pull_request.head.sha",
	"github.head_ref",
}

// prHeadRepoContexts lists expression substrings that reference the PR's head
// repository. Using any of these as the repository: in an actions/checkout
// step within a pull_request_target workflow fetches code from the fork.
var prHeadRepoContexts = []string{
	"github.event.pull_request.head.repo.full_name",
}

// checkPullRequestTarget returns warnings for workflows that use the
// pull_request_target trigger and check out user-controlled code via
// actions/checkout with a PR head ref or repository.
func checkPullRequestTarget(root *yaml.Node, source string) []Warning {
	if root.Kind == 0 || len(root.Content) == 0 {
		return nil
	}
	doc := root.Content[0]
	if doc.Kind != yaml.MappingNode {
		return nil
	}
	if !hasPullRequestTarget(doc) {
		return nil
	}
	return findDangerousCheckouts(doc, source)
}

// hasPullRequestTarget reports whether the document's on: field includes
// pull_request_target. Handles all three YAML forms of the on: key:
// scalar, sequence, and mapping.
func hasPullRequestTarget(doc *yaml.Node) bool {
	for i := 0; i+1 < len(doc.Content); i += 2 {
		if doc.Content[i].Value != "on" {
			continue
		}
		val := doc.Content[i+1]
		switch val.Kind {
		case yaml.ScalarNode:
			return val.Value == "pull_request_target"
		case yaml.SequenceNode:
			for _, item := range val.Content {
				if item.Kind == yaml.ScalarNode && item.Value == "pull_request_target" {
					return true
				}
			}
		case yaml.MappingNode:
			for j := 0; j+1 < len(val.Content); j += 2 {
				if val.Content[j].Value == "pull_request_target" {
					return true
				}
			}
		}
	}
	return false
}

// findDangerousCheckouts walks all jobs looking for actions/checkout steps
// that supply a user-controlled ref: or repository:.
func findDangerousCheckouts(doc *yaml.Node, source string) []Warning {
	var jobsNode *yaml.Node
	for i := 0; i+1 < len(doc.Content); i += 2 {
		if doc.Content[i].Value == "jobs" {
			jobsNode = doc.Content[i+1]
			break
		}
	}
	if jobsNode == nil || jobsNode.Kind != yaml.MappingNode {
		return nil
	}

	var warnings []Warning
	for i := 0; i+1 < len(jobsNode.Content); i += 2 {
		jobVal := jobsNode.Content[i+1]
		if jobVal.Kind != yaml.MappingNode {
			continue
		}
		for j := 0; j+1 < len(jobVal.Content); j += 2 {
			if jobVal.Content[j].Value != "steps" {
				continue
			}
			steps := jobVal.Content[j+1]
			if steps.Kind != yaml.SequenceNode {
				continue
			}
			for _, step := range steps.Content {
				warnings = append(warnings, checkCheckoutStep(step, source)...)
			}
		}
	}
	return warnings
}

// checkCheckoutStep returns warnings if the step is an actions/checkout with a
// user-controlled ref: or repository:.
func checkCheckoutStep(step *yaml.Node, source string) []Warning {
	if step.Kind != yaml.MappingNode {
		return nil
	}

	var isCheckout bool
	var withNode *yaml.Node

	for i := 0; i+1 < len(step.Content); i += 2 {
		key := step.Content[i]
		val := step.Content[i+1]
		switch key.Value {
		case "uses":
			if val.Kind == yaml.ScalarNode && isCheckoutAction(val.Value) {
				isCheckout = true
			}
		case "with":
			if val.Kind == yaml.MappingNode {
				withNode = val
			}
		}
	}

	if !isCheckout || withNode == nil {
		return nil
	}

	var warnings []Warning
	for i := 0; i+1 < len(withNode.Content); i += 2 {
		key := withNode.Content[i]
		val := withNode.Content[i+1]
		if val.Kind != yaml.ScalarNode {
			continue
		}
		switch key.Value {
		case "ref":
			if w := checkWithParam(val.Value, val.Line, source, prHeadRefContexts,
				"pull_request_target: checkout of PR head ref (%s) runs untrusted code with write access and secrets"); w != nil {
				warnings = append(warnings, *w)
			}
		case "repository":
			if w := checkWithParam(val.Value, val.Line, source, prHeadRepoContexts,
				"pull_request_target: checkout from PR fork repository (%s) runs untrusted code with write access and secrets"); w != nil {
				warnings = append(warnings, *w)
			}
		}
	}
	return warnings
}

func checkWithParam(value string, line int, source string, contexts []string, msgFmt string) *Warning {
	lower := strings.ToLower(value)
	for _, ctx := range contexts {
		if strings.Contains(lower, ctx) {
			return &Warning{
				File:    source,
				Line:    line,
				Message: fmt.Sprintf(msgFmt, value),
			}
		}
	}
	return nil
}

// isCheckoutAction reports whether a uses: value refers to actions/checkout.
func isCheckoutAction(uses string) bool {
	lower := strings.ToLower(uses)
	return lower == "actions/checkout" ||
		strings.HasPrefix(lower, "actions/checkout@")
}

package lint

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// checkPermissions checks a parsed workflow file for missing or overly broad
// GITHUB_TOKEN permissions. Returns no warnings for composite action files,
// which are identified by the absence of a top-level jobs: key.
func checkPermissions(root *yaml.Node, source string) []Warning {
	if root.Kind == 0 || len(root.Content) == 0 {
		return nil
	}
	doc := root.Content[0]
	if doc.Kind != yaml.MappingNode {
		return nil
	}

	var permNode *yaml.Node
	var permLine int
	var jobsNode *yaml.Node

	for i := 0; i+1 < len(doc.Content); i += 2 {
		key := doc.Content[i]
		val := doc.Content[i+1]
		switch key.Value {
		case "permissions":
			permNode = val
			permLine = key.Line
		case "jobs":
			jobsNode = val
		}
	}

	// Only check workflow files (those with a jobs: section).
	if jobsNode == nil {
		return nil
	}

	var warnings []Warning

	switch {
	case permNode == nil:
		warnings = append(warnings, Warning{
			File:    source,
			Line:    1,
			Message: "no top-level permissions declared; GITHUB_TOKEN defaults to the repository's base permissions (use permissions: {} for least privilege)",
		})
	case permNode.Kind == yaml.ScalarNode && permNode.Value == "write-all":
		warnings = append(warnings, Warning{
			File:    source,
			Line:    permLine,
			Message: "permissions: write-all grants GITHUB_TOKEN full write access to all scopes",
		})
	}

	// Also flag write-all at the job level regardless of workflow-level settings.
	if jobsNode.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(jobsNode.Content); i += 2 {
			jobName := jobsNode.Content[i].Value
			jobVal := jobsNode.Content[i+1]
			if jobVal.Kind != yaml.MappingNode {
				continue
			}
			for j := 0; j+1 < len(jobVal.Content); j += 2 {
				if jobVal.Content[j].Value == "permissions" {
					perm := jobVal.Content[j+1]
					if perm.Kind == yaml.ScalarNode && perm.Value == "write-all" {
						warnings = append(warnings, Warning{
							File:    source,
							Line:    perm.Line,
							Message: fmt.Sprintf("job %q uses permissions: write-all", jobName),
						})
					}
				}
			}
		}
	}

	return warnings
}

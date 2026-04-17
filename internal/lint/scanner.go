package lint

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

var shaPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)

type Warning struct {
	File    string
	Line    int
	Uses    string // non-empty for pinning warnings; used by ignore list filtering
	Message string // full human-readable warning text
}

// FindActionFiles returns all workflow and composite action YAML files under repoRoot.
func FindActionFiles(repoRoot string) ([]string, error) {
	var files []string

	for _, ext := range []string{"yml", "yaml"} {
		matches, err := filepath.Glob(filepath.Join(repoRoot, ".github", "workflows", "*."+ext))
		if err != nil {
			return nil, err
		}
		files = append(files, matches...)
	}

	// Also find composite actions nested under .github/actions/
	err := filepath.WalkDir(filepath.Join(repoRoot, ".github", "actions"), func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable dirs
		}
		if !d.IsDir() && (d.Name() == "action.yml" || d.Name() == "action.yaml") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	return files, nil
}

// CheckFile parses a workflow or composite action file and returns warnings for
// any `uses:` values that are not pinned to a full commit SHA.
// Warnings matching ignore are suppressed, but ignored actions are still returned
// in allUses so callers can recurse into them.
func CheckFile(path string, ignore *IgnoreList, disabled DisabledChecks) ([]Warning, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	warnings, _, err := parseContent(data, path, disabled)
	if err != nil {
		return nil, err
	}
	return filterWarnings(warnings, ignore), nil
}

// ExternalUsesFromFile returns all external action refs used in the file.
func ExternalUsesFromFile(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	_, uses, err := parseContent(data, path, DisabledChecks{})
	return uses, err
}

// parseContent parses YAML bytes and returns both warnings and all external uses.
func parseContent(data []byte, source string, disabled DisabledChecks) ([]Warning, []string, error) {
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, nil, fmt.Errorf("parsing %s: %w", source, err)
	}
	if root.Kind == 0 {
		return nil, nil, nil
	}
	var warnings []Warning
	var allUses []string
	walkNode(&root, source, &warnings, &allUses, disabled)
	if !disabled.Permissions {
		warnings = append(warnings, checkPermissions(&root, source)...)
	}
	return warnings, allUses, nil
}

func walkNode(node *yaml.Node, file string, warnings *[]Warning, allUses *[]string, disabled DisabledChecks) {
	if node.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(node.Content); i += 2 {
			key := node.Content[i]
			value := node.Content[i+1]
			if value.Kind == yaml.ScalarNode {
				switch key.Value {
				case "uses":
					u := value.Value
					if allUses != nil && isExternalUses(u) {
						*allUses = append(*allUses, u)
					}
					if !disabled.Pins {
						if w := checkUses(u, file, value.Line); w != nil {
							*warnings = append(*warnings, *w)
						}
					}
				case "run":
					if !disabled.Injections {
						lineOffset := 0
						if value.Style == yaml.LiteralStyle || value.Style == yaml.FoldedStyle {
							lineOffset = 1
						}
						*warnings = append(*warnings, checkRunInjection(value.Value, file, value.Line, lineOffset)...)
					}
				}
			}
			walkNode(value, file, warnings, allUses, disabled)
		}
	} else {
		for _, child := range node.Content {
			walkNode(child, file, warnings, allUses, disabled)
		}
	}
}

func isExternalUses(uses string) bool {
	return !strings.HasPrefix(uses, "./") && !strings.HasPrefix(uses, "docker://")
}

func filterWarnings(warnings []Warning, ignore *IgnoreList) []Warning {
	if ignore == nil || len(ignore.patterns) == 0 {
		return warnings
	}
	filtered := warnings[:0]
	for _, w := range warnings {
		if !ignore.IsIgnored(w.Uses) {
			filtered = append(filtered, w)
		}
	}
	return filtered
}

// checkUses returns a Warning if the uses value is not pinned to a SHA.
// Local actions (./path) and docker images are ignored.
func checkUses(uses, file string, line int) *Warning {
	if !isExternalUses(uses) {
		return nil
	}

	parts := strings.SplitN(uses, "@", 2)
	if len(parts) != 2 {
		// No ref at all — also worth warning about
		return &Warning{File: file, Line: line, Uses: uses, Message: "action not pinned to a SHA: " + uses}
	}

	ref := parts[1]
	if !shaPattern.MatchString(ref) {
		return &Warning{File: file, Line: line, Uses: uses, Message: "action not pinned to a SHA: " + uses}
	}

	return nil
}

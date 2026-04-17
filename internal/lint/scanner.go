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
	File string
	Line int
	Uses string
}

// FindActionFiles returns all workflow and composite action YAML files under repoRoot.
func FindActionFiles(repoRoot string) ([]string, error) {
	var files []string

	patterns := []string{
		".github/workflows/*.yml",
		".github/workflows/*.yaml",
	}

	for _, pattern := range patterns {
		matches, err := filepath.Glob(filepath.Join(repoRoot, pattern))
		if err != nil {
			return nil, err
		}
		files = append(files, matches...)
	}

	// Also find composite actions nested under .github/actions/
	for _, name := range []string{"action.yml", "action.yaml"} {
		err := filepath.WalkDir(filepath.Join(repoRoot, ".github", "actions"), func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil // skip unreadable dirs
			}
			if !d.IsDir() && d.Name() == name {
				files = append(files, path)
			}
			return nil
		})
		if err != nil && !os.IsNotExist(err) {
			return nil, err
		}
	}

	return files, nil
}

// CheckFile parses a workflow or composite action file and returns warnings for
// any `uses:` values that are not pinned to a full commit SHA.
func CheckFile(path string) ([]Warning, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	warnings, _, err := parseContent(data, path)
	return warnings, err
}

// ExternalUsesFromFile returns all external action refs used in the file.
func ExternalUsesFromFile(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	_, uses, err := parseContent(data, path)
	return uses, err
}

// parseContent parses YAML bytes and returns both warnings and all external uses.
func parseContent(data []byte, source string) ([]Warning, []string, error) {
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, nil, fmt.Errorf("parsing %s: %w", source, err)
	}
	if root.Kind == 0 {
		return nil, nil, nil
	}
	var warnings []Warning
	var allUses []string
	walkNode(&root, source, &warnings, &allUses)
	return warnings, allUses, nil
}

func walkNode(node *yaml.Node, file string, warnings *[]Warning, allUses *[]string) {
	if node.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(node.Content); i += 2 {
			key := node.Content[i]
			value := node.Content[i+1]
			if key.Value == "uses" && value.Kind == yaml.ScalarNode {
				u := value.Value
				if allUses != nil && isExternalUses(u) {
					*allUses = append(*allUses, u)
				}
				if w := checkUses(u, file, value.Line); w != nil {
					*warnings = append(*warnings, *w)
				}
			}
			walkNode(value, file, warnings, allUses)
		}
	} else {
		for _, child := range node.Content {
			walkNode(child, file, warnings, allUses)
		}
	}
}

func isExternalUses(uses string) bool {
	return !strings.HasPrefix(uses, "./") && !strings.HasPrefix(uses, "docker://")
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
		return &Warning{File: file, Line: line, Uses: uses}
	}

	ref := parts[1]
	if !shaPattern.MatchString(ref) {
		return &Warning{File: file, Line: line, Uses: uses}
	}

	return nil
}

package lint

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

const ignoreFileName = ".gh-lint-ignore"

// IgnoreList holds action patterns that are exempt from warnings.
// A pattern can be a full uses ref ("actions/checkout@v4") or just the action
// name ("actions/checkout"), which matches any ref of that action.
type IgnoreList struct {
	patterns map[string]bool
}

// LoadIgnoreFile reads the .gh-lint-ignore file from repoRoot.
// Returns an empty IgnoreList if the file doesn't exist.
func LoadIgnoreFile(repoRoot string) (*IgnoreList, error) {
	path := filepath.Join(repoRoot, ignoreFileName)
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return &IgnoreList{patterns: map[string]bool{}}, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	il := &IgnoreList{patterns: map[string]bool{}}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if i := strings.Index(line, "#"); i >= 0 {
			line = strings.TrimSpace(line[:i])
		}
		if line != "" {
			il.patterns[line] = true
		}
	}
	return il, sc.Err()
}

// IsIgnored reports whether the uses value matches any pattern in the list.
func (il *IgnoreList) IsIgnored(uses string) bool {
	if il == nil || len(il.patterns) == 0 {
		return false
	}
	if il.patterns[uses] {
		return true
	}
	// Also match on the action name without the ref.
	if i := strings.Index(uses, "@"); i >= 0 {
		return il.patterns[uses[:i]]
	}
	return false
}

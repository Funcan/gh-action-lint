package lint

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

// usesLineRe matches a line containing a `uses:` directive.
// Groups: (1) prefix  (2) action  (3) ref  (4) trailing whitespace/comment
var usesLineRe = regexp.MustCompile(`^(\s*-?\s*uses:\s+)(\S+)@(\S+?)(\s*(?:#.*)?)$`)

// FixResult describes one substitution made (or attempted) in a file.
type FixResult struct {
	Line int
	From string // original uses value, e.g. "actions/checkout@v4"
	To   string // new uses value, e.g. "actions/checkout@abc123 # v4" (empty on error)
	Err  error  // non-nil if the ref could not be resolved; line was left unchanged
}

// FixFile replaces unpinned action refs in path with their resolved SHAs, adding
// the original ref as a comment. Actions matching ignore are left untouched.
// Resolution failures are reported as FixResults with Err set rather than
// aborting the whole file.
func FixFile(path string, ignore *IgnoreList, resolver *Resolver) ([]FixResult, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	fi, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(string(data), "\n")
	var results []FixResult
	anyFixed := false

	for i, line := range lines {
		newLine, from, to, fixErr := fixLine(line, ignore, resolver)
		if fixErr != nil {
			results = append(results, FixResult{Line: i + 1, From: from, Err: fixErr})
			continue // leave original line in place
		}
		if newLine != line {
			lines[i] = newLine
			results = append(results, FixResult{Line: i + 1, From: from, To: to})
			anyFixed = true
		}
	}

	if anyFixed {
		if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), fi.Mode()); err != nil {
			return results, err
		}
	}

	return results, nil
}

// fixLine processes one line and returns the (possibly updated) line plus the
// from/to uses strings. If nothing needed changing it returns the original line
// with empty from/to. If resolution fails it returns the original line, the
// from value, and a non-nil error.
func fixLine(line string, ignore *IgnoreList, resolver *Resolver) (fixed, from, to string, err error) {
	m := usesLineRe.FindStringSubmatch(line)
	if m == nil {
		return line, "", "", nil
	}

	prefix := m[1] // e.g. "      - uses: "
	action := m[2] // e.g. "actions/checkout"
	ref := m[3]    // e.g. "v4"
	// m[4] is any trailing whitespace/comment — discarded and replaced

	// Already pinned to a SHA — nothing to do.
	if shaPattern.MatchString(ref) {
		return line, "", "", nil
	}

	uses := action + "@" + ref

	if !isExternalUses(uses) {
		return line, "", "", nil
	}

	if ignore.IsIgnored(uses) {
		return line, "", "", nil
	}

	owner, repo, _, gitRef := splitUses(uses)
	if owner == "" {
		return line, uses, "", fmt.Errorf("cannot parse uses value: %s", uses)
	}

	sha, err := resolver.Resolve(owner, repo, gitRef)
	if err != nil {
		return line, uses, "", fmt.Errorf("resolving %s: %w", uses, err)
	}

	newUses := action + "@" + sha + " # " + ref
	return prefix + newUses, uses, newUses, nil
}

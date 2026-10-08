package lint

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// versionRe matches release tags such as v4, v4.1 and v4.1.2 (the "v" is
// optional). Pre-release tags like v5.0.0-beta deliberately do not match.
var versionRe = regexp.MustCompile(`^(v?)(\d+)(?:\.(\d+))?(?:\.(\d+))?$`)

// commentVersionRe extracts the version from a pin comment like "# v4.1.2".
var commentVersionRe = regexp.MustCompile(`#\s*(\S+)`)

// version is a parsed release tag.
type version struct {
	name      string // original tag, e.g. "v4.1"
	prefix    string // "v" or ""
	nums      [3]int // major, minor, patch (missing components are 0)
	precision int    // number of components present in the tag (1-3)
}

func parseVersion(tag string) (version, bool) {
	m := versionRe.FindStringSubmatch(tag)
	if m == nil {
		return version{}, false
	}
	v := version{name: tag, prefix: m[1]}
	for i, s := range m[2:] {
		if s == "" {
			break
		}
		v.nums[i], _ = strconv.Atoi(s)
		v.precision++
	}
	return v, true
}

// compare returns -1, 0 or 1. Missing components compare as 0, so v4 == v4.0.0.
func (v version) compare(o version) int {
	for i := range v.nums {
		if v.nums[i] != o.nums[i] {
			if v.nums[i] < o.nums[i] {
				return -1
			}
			return 1
		}
	}
	return 0
}

// UpdateOptions controls which actions UpdateFile updates and how far.
type UpdateOptions struct {
	NoMajorBump bool   // stay on the current major version
	Action      string // if set, only update this action (owner/repo or owner/repo/path)
}

// UpdateResult describes one update made, skipped with a warning, or failed.
type UpdateResult struct {
	Line    int
	From    string // e.g. "actions/checkout@v4"
	To      string // e.g. "actions/checkout@v5" (empty if not updated)
	Warning string // set when a newer major version was skipped due to NoMajorBump
	Err     error
}

// UpdateFile updates each external action in path to its latest release tag,
// pinned to the tag's commit SHA with the tag as a comment. The current version
// is taken from the ref, or from the pin comment if the ref is a SHA. Refs that
// are not release versions (e.g. branches) and ignored actions are left alone.
func UpdateFile(path string, ignore *IgnoreList, resolver *Resolver, opts UpdateOptions) ([]UpdateResult, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	fi, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(string(data), "\n")
	var results []UpdateResult
	anyUpdated := false
	for i, line := range lines {
		newLine, r := updateLine(line, ignore, resolver, opts)
		if r == nil {
			continue
		}
		r.Line = i + 1
		results = append(results, *r)
		if newLine != line {
			lines[i] = newLine
			anyUpdated = true
		}
	}

	if anyUpdated {
		if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), fi.Mode()); err != nil {
			return results, err
		}
	}
	return results, nil
}

// updateLine returns the (possibly updated) line and a result, or a nil result
// if the line needed no attention.
func updateLine(line string, ignore *IgnoreList, resolver *Resolver, opts UpdateOptions) (string, *UpdateResult) {
	m := usesLineRe.FindStringSubmatch(line)
	if m == nil {
		return line, nil
	}
	prefix, action, ref, trailing := m[1], m[2], m[3], m[4]

	uses := action + "@" + ref
	if !isExternalUses(uses) || ignore.IsIgnored(uses) {
		return line, nil
	}
	owner, repo, _, _ := splitUses(uses)
	if owner == "" {
		return line, nil
	}
	if opts.Action != "" && opts.Action != action && opts.Action != owner+"/"+repo {
		return line, nil
	}

	pinned := shaPattern.MatchString(ref)
	currentTag := ref
	if pinned {
		cm := commentVersionRe.FindStringSubmatch(trailing)
		if cm == nil {
			return line, nil // pinned with no version comment; nothing to compare against
		}
		currentTag = cm[1]
	}
	current, ok := parseVersion(currentTag)
	if !ok {
		return line, nil
	}

	from := action + "@" + currentTag
	tags, err := resolver.Tags(owner, repo)
	if err != nil {
		return line, &UpdateResult{From: from, Err: fmt.Errorf("listing tags for %s/%s: %w", owner, repo, err)}
	}

	target, newestMajor := pickUpdate(current, tags, opts.NoMajorBump)

	var warning string
	if newestMajor != nil {
		warning = fmt.Sprintf("%s/%s %s is available (not bumping major version)", owner, repo, newestMajor.name)
	}

	if target == nil || target.compare(current) < 0 || (target.name == currentTag && !pinned) {
		if warning == "" {
			return line, nil
		}
		return line, &UpdateResult{From: from, Warning: warning}
	}

	sha, err := resolver.Resolve(owner, repo, target.name)
	if err != nil {
		return line, &UpdateResult{From: from, Err: fmt.Errorf("resolving %s@%s: %w", action, target.name, err)}
	}
	if pinned && sha == ref && target.name == currentTag {
		if warning == "" {
			return line, nil
		}
		return line, &UpdateResult{From: from, Warning: warning}
	}

	return prefix + action + "@" + sha + " # " + target.name, &UpdateResult{
		From:    from,
		To:      action + "@" + target.name,
		Warning: warning,
	}
}

// pickUpdate chooses the tag to update to: the newest release (restricted to
// the current major if noMajorBump), expressed at the current tag's precision
// when such a tag exists (so v4 moves to v5, not v5.0.1). If noMajorBump hides
// a newer major release, that release is returned as newestMajor.
func pickUpdate(current version, tags []string, noMajorBump bool) (target, newestMajor *version) {
	var best, newest *version
	byName := make(map[string]bool, len(tags))
	for _, t := range tags {
		v, ok := parseVersion(t)
		if !ok {
			continue
		}
		byName[t] = true
		if newest == nil || v.compare(*newest) > 0 || (v.compare(*newest) == 0 && v.precision > newest.precision) {
			newest = &v
		}
		if noMajorBump && v.nums[0] != current.nums[0] {
			continue
		}
		if best == nil || v.compare(*best) > 0 || (v.compare(*best) == 0 && v.precision > best.precision) {
			best = &v
		}
	}

	if noMajorBump && newest != nil && newest.nums[0] > current.nums[0] {
		newestMajor = newest
	}
	if best == nil {
		return nil, newestMajor
	}

	if best.precision > current.precision {
		parts := make([]string, current.precision)
		for i := range parts {
			parts[i] = strconv.Itoa(best.nums[i])
		}
		name := best.prefix + strings.Join(parts, ".")
		if byName[name] {
			v, _ := parseVersion(name)
			return &v, newestMajor
		}
	}
	return best, newestMajor
}

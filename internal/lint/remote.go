package lint

import (
	"fmt"
	"io"
	"net/http"
	"strings"
)

const rawGitHubBase = "https://raw.githubusercontent.com"

// CheckRecursive fetches each action in startUses and checks for unpinned refs,
// recursing transitively into any further actions they use. GITHUB_TOKEN is used
// for authentication if non-empty. Already-visited refs are skipped.
// Warnings matching ignore are suppressed, but ignored actions are still traversed.
func CheckRecursive(startUses []string, token string, ignore *IgnoreList, disabled DisabledChecks) ([]Warning, error) {
	visited := make(map[string]bool)
	queue := startUses

	var warnings []Warning
	for len(queue) > 0 {
		ref := queue[0]
		queue = queue[1:]

		if visited[ref] {
			continue
		}
		visited[ref] = true

		content, err := fetchActionYAML(ref, token)
		if err != nil {
			// Many actions are not composite (JS/Docker) and have no action.yml steps —
			// silently skip them.
			continue
		}

		ws, allUses, err := parseContent(content, ref, disabled)
		if err != nil {
			continue
		}
		warnings = append(warnings, filterWarnings(ws, ignore)...)

		// Always queue all discovered uses, even if the action itself is ignored,
		// so we recurse into the full dependency graph.
		for _, u := range allUses {
			if !visited[u] {
				queue = append(queue, u)
			}
		}
	}

	return warnings, nil
}

// fetchActionYAML fetches the action.yml (or action.yaml) for an external uses ref
// from raw.githubusercontent.com.
func fetchActionYAML(uses, token string) ([]byte, error) {
	owner, repo, subpath, ref := splitUses(uses)
	if owner == "" {
		return nil, fmt.Errorf("cannot parse uses: %s", uses)
	}

	client := &http.Client{}
	for _, filename := range []string{"action.yml", "action.yaml"} {
		var rawURL string
		if subpath != "" {
			rawURL = fmt.Sprintf("%s/%s/%s/%s/%s/%s", rawGitHubBase, owner, repo, ref, subpath, filename)
		} else {
			rawURL = fmt.Sprintf("%s/%s/%s/%s/%s", rawGitHubBase, owner, repo, ref, filename)
		}

		req, err := http.NewRequest(http.MethodGet, rawURL, nil)
		if err != nil {
			continue
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}

		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			return io.ReadAll(resp.Body)
		}
	}

	return nil, fmt.Errorf("action file not found for %s", uses)
}

// splitUses parses "owner/repo@ref" or "owner/repo/subpath@ref" into its parts.
func splitUses(uses string) (owner, repo, subpath, ref string) {
	at := strings.SplitN(uses, "@", 2)
	if len(at) != 2 {
		return
	}
	ref = at[1]
	segments := strings.SplitN(at[0], "/", 3)
	if len(segments) < 2 {
		return
	}
	owner = segments[0]
	repo = segments[1]
	if len(segments) == 3 {
		subpath = segments[2]
	}
	return
}

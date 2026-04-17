package lint

import (
	"encoding/json"
	"fmt"
	"net/http"
)

const githubAPIBase = "https://api.github.com"

// Resolver resolves git refs to commit SHAs using the GitHub API, caching results.
type Resolver struct {
	token string
	cache map[string]string
}

func NewResolver(token string) *Resolver {
	return &Resolver{token: token, cache: make(map[string]string)}
}

// Resolve returns the full commit SHA for the given owner/repo at ref.
func (r *Resolver) Resolve(owner, repo, ref string) (string, error) {
	key := owner + "/" + repo + "@" + ref
	if sha, ok := r.cache[key]; ok {
		return sha, nil
	}

	url := fmt.Sprintf("%s/repos/%s/%s/commits/%s", githubAPIBase, owner, repo, ref)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	if r.token != "" {
		req.Header.Set("Authorization", "Bearer "+r.token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub API returned %d for %s/%s@%s", resp.StatusCode, owner, repo, ref)
	}

	var result struct {
		SHA string `json:"sha"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	if result.SHA == "" {
		return "", fmt.Errorf("no SHA in response for %s/%s@%s", owner, repo, ref)
	}

	r.cache[key] = result.SHA
	return result.SHA, nil
}

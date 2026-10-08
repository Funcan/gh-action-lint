package lint

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const githubAPIBase = "https://api.github.com"

const (
	maxAttempts  = 6
	initialDelay = time.Second
	// maxRateLimitWait caps how long we will sleep for a single rate-limit
	// reset; beyond this we give up rather than appear to hang.
	maxRateLimitWait = 5 * time.Minute
)

// linkNextRe extracts the "next" URL from a GitHub API Link header.
var linkNextRe = regexp.MustCompile(`<([^>]+)>;\s*rel="next"`)

// Resolver resolves git refs to commit SHAs and lists tags using the GitHub
// API, caching results. Rate-limited requests are retried with backoff.
type Resolver struct {
	token    string
	baseURL  string
	sleep    func(time.Duration)
	cache    map[string]string
	tagCache map[string][]string
}

func NewResolver(token string) *Resolver {
	return &Resolver{
		token:    token,
		baseURL:  githubAPIBase,
		sleep:    time.Sleep,
		cache:    make(map[string]string),
		tagCache: make(map[string][]string),
	}
}

// get performs an authenticated GET against the GitHub API and decodes the
// JSON response into v. It returns the response's "next" page URL, if any.
// Rate-limit responses (403/429) are retried with backoff.
func (r *Resolver) get(url string, v any) (next string, err error) {
	delay := initialDelay
	for attempt := 1; ; attempt++ {
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

		if wait, limited := rateLimitWait(resp, delay); limited {
			resp.Body.Close()
			if attempt >= maxAttempts {
				return "", fmt.Errorf("GitHub API rate limit exceeded for %s after %d attempts", url, attempt)
			}
			if wait > maxRateLimitWait {
				return "", fmt.Errorf("GitHub API rate limit exceeded for %s (resets in %s)", url, wait.Round(time.Second))
			}
			r.sleep(wait)
			delay *= 2
			continue
		}

		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("GitHub API returned %d for %s", resp.StatusCode, url)
		}
		if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
			return "", err
		}
		if m := linkNextRe.FindStringSubmatch(resp.Header.Get("Link")); m != nil {
			next = m[1]
		}
		return next, nil
	}
}

// rateLimitWait reports whether resp is a rate-limit response and, if so, how
// long to wait before retrying. It honours Retry-After (secondary limits) and
// x-ratelimit-reset (primary limits), falling back to the backoff delay.
func rateLimitWait(resp *http.Response, backoff time.Duration) (time.Duration, bool) {
	if resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusTooManyRequests {
		return 0, false
	}
	if s := resp.Header.Get("Retry-After"); s != "" {
		if secs, err := strconv.Atoi(s); err == nil {
			return time.Duration(secs) * time.Second, true
		}
	}
	if resp.Header.Get("X-RateLimit-Remaining") == "0" {
		if reset, err := strconv.ParseInt(resp.Header.Get("X-RateLimit-Reset"), 10, 64); err == nil {
			return max(time.Until(time.Unix(reset, 0)), 0) + time.Second, true
		}
		return backoff, true
	}
	// 429 is always a rate limit; a bare 403 is a genuine permission error.
	if resp.StatusCode == http.StatusTooManyRequests {
		return backoff, true
	}
	return 0, false
}

// Resolve returns the full commit SHA for the given owner/repo at ref.
func (r *Resolver) Resolve(owner, repo, ref string) (string, error) {
	key := owner + "/" + repo + "@" + ref
	if sha, ok := r.cache[key]; ok {
		return sha, nil
	}

	var result struct {
		SHA string `json:"sha"`
	}
	url := fmt.Sprintf("%s/repos/%s/%s/commits/%s", r.baseURL, owner, repo, ref)
	if _, err := r.get(url, &result); err != nil {
		return "", err
	}
	if result.SHA == "" {
		return "", fmt.Errorf("no SHA in response for %s/%s@%s", owner, repo, ref)
	}

	r.cache[key] = result.SHA
	return result.SHA, nil
}

// Tags returns the names of all tags in owner/repo.
func (r *Resolver) Tags(owner, repo string) ([]string, error) {
	key := owner + "/" + repo
	if tags, ok := r.tagCache[key]; ok {
		return tags, nil
	}

	var tags []string
	url := fmt.Sprintf("%s/repos/%s/%s/git/matching-refs/tags/", r.baseURL, owner, repo)
	for url != "" {
		var refs []struct {
			Ref string `json:"ref"`
		}
		next, err := r.get(url, &refs)
		if err != nil {
			return nil, err
		}
		for _, ref := range refs {
			tags = append(tags, strings.TrimPrefix(ref.Ref, "refs/tags/"))
		}
		url = next
	}

	r.tagCache[key] = tags
	return tags, nil
}

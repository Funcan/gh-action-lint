package lint

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	shaOld = "1111111111111111111111111111111111111111"
	shaV4  = "4444444444444444444444444444444444444444"
	shaV5  = "5555555555555555555555555555555555555555"
	shaV41 = "4141414141414141414141414141414141414141"
)

func testResolver() *Resolver {
	return &Resolver{
		cache: map[string]string{
			"actions/checkout@v4":     shaV4,
			"actions/checkout@v5":     shaV5,
			"actions/checkout@v4.1.0": shaV41,
			"actions/checkout@v5.0.1": shaV5,
		},
		tagCache: map[string][]string{
			"actions/checkout": {"v3", "v3.6.0", "v4", "v4.0.0", "v4.1.0", "v5", "v5.0.0", "v5.0.1", "v6.0.0-beta", "latest"},
		},
	}
}

func TestPickUpdate(t *testing.T) {
	tags := testResolver().tagCache["actions/checkout"]
	tests := []struct {
		current     string
		noMajor     bool
		wantTarget  string
		wantNewestM string
	}{
		{"v4", false, "v5", ""},
		{"v4.0.0", false, "v5.0.1", ""},
		{"v4", true, "v4", "v5.0.1"},
		{"v4.0.0", true, "v4.1.0", "v5.0.1"},
		{"v5", true, "v5", ""},
		{"v3.6", false, "v5.0.1", ""}, // no v5.0 tag, so fall back to the full tag
	}
	for _, tt := range tests {
		cur, _ := parseVersion(tt.current)
		target, newest := pickUpdate(cur, tags, tt.noMajor)
		if target == nil || target.name != tt.wantTarget {
			t.Errorf("%s noMajor=%v: target = %v, want %s", tt.current, tt.noMajor, target, tt.wantTarget)
		}
		gotNewest := ""
		if newest != nil {
			gotNewest = newest.name
		}
		if gotNewest != tt.wantNewestM {
			t.Errorf("%s noMajor=%v: newestMajor = %q, want %q", tt.current, tt.noMajor, gotNewest, tt.wantNewestM)
		}
	}
}

func TestUpdateLine(t *testing.T) {
	tests := []struct {
		name        string
		line        string
		opts        UpdateOptions
		wantLine    string
		wantTo      string
		wantWarning bool
		wantNil     bool
	}{
		{
			name:     "unpinned major",
			line:     "      - uses: actions/checkout@v4",
			wantLine: "      - uses: actions/checkout@" + shaV5 + " # v5",
			wantTo:   "actions/checkout@v5",
		},
		{
			name:     "pinned uses comment version",
			line:     "      - uses: actions/checkout@" + shaOld + " # v4.0.0",
			wantLine: "      - uses: actions/checkout@" + shaV5 + " # v5.0.1",
			wantTo:   "actions/checkout@v5.0.1",
		},
		{
			name:        "no major bump warns",
			line:        "      - uses: actions/checkout@" + shaOld + " # v4.0.0",
			opts:        UpdateOptions{NoMajorBump: true},
			wantLine:    "      - uses: actions/checkout@" + shaV41 + " # v4.1.0",
			wantTo:      "actions/checkout@v4.1.0",
			wantWarning: true,
		},
		{
			name:        "pinned floating tag moved",
			line:        "      - uses: actions/checkout@" + shaOld + " # v4",
			opts:        UpdateOptions{NoMajorBump: true},
			wantLine:    "      - uses: actions/checkout@" + shaV4 + " # v4",
			wantTo:      "actions/checkout@v4",
			wantWarning: true,
		},
		{
			name:        "unpinned already latest in major only warns",
			line:        "      - uses: actions/checkout@v4",
			opts:        UpdateOptions{NoMajorBump: true},
			wantLine:    "      - uses: actions/checkout@v4",
			wantWarning: true,
		},
		{
			name:    "up to date",
			line:    "      - uses: actions/checkout@" + shaV5 + " # v5",
			wantNil: true,
		},
		{
			name:    "other action filtered out",
			line:    "      - uses: actions/checkout@v4",
			opts:    UpdateOptions{Action: "actions/cache"},
			wantNil: true,
		},
		{
			name:     "action filter matches",
			line:     "      - uses: actions/checkout@v4",
			opts:     UpdateOptions{Action: "actions/checkout"},
			wantLine: "      - uses: actions/checkout@" + shaV5 + " # v5",
			wantTo:   "actions/checkout@v5",
		},
		{name: "branch ref", line: "      - uses: actions/checkout@main", wantNil: true},
		{name: "pinned without comment", line: "      - uses: actions/checkout@" + shaOld, wantNil: true},
		{name: "local", line: "      - uses: ./local@v1", wantNil: true},
		{name: "not uses", line: "      run: echo hi", wantNil: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, r := updateLine(tt.line, nil, testResolver(), tt.opts)
			if tt.wantNil {
				if r != nil || got != tt.line {
					t.Fatalf("expected no change, got %q, %+v", got, r)
				}
				return
			}
			if r == nil {
				t.Fatal("expected result")
			}
			if r.Err != nil {
				t.Fatal(r.Err)
			}
			if got != tt.wantLine {
				t.Errorf("line = %q, want %q", got, tt.wantLine)
			}
			if r.To != tt.wantTo {
				t.Errorf("To = %q, want %q", r.To, tt.wantTo)
			}
			if (r.Warning != "") != tt.wantWarning {
				t.Errorf("Warning = %q, wantWarning %v", r.Warning, tt.wantWarning)
			}
		})
	}
}

func TestUpdateLineIgnored(t *testing.T) {
	il := &IgnoreList{patterns: map[string]bool{"actions/checkout": true}}
	line := "      - uses: actions/checkout@v4"
	if got, r := updateLine(line, il, testResolver(), UpdateOptions{}); r != nil || got != line {
		t.Errorf("expected ignored action to be untouched")
	}
}

func TestUpdateFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ci.yml")
	content := "jobs:\n  build:\n    steps:\n      - uses: actions/checkout@v4\n      - run: echo hi\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	results, err := UpdateFile(path, nil, testResolver(), UpdateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Line != 4 {
		t.Fatalf("unexpected results: %+v", results)
	}
	data, _ := os.ReadFile(path)
	want := strings.Replace(content, "actions/checkout@v4", "actions/checkout@"+shaV5+" # v5", 1)
	if string(data) != want {
		t.Errorf("got:\n%s\nwant:\n%s", data, want)
	}
}

func TestResolverRetriesOnRateLimit(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		switch calls {
		case 1:
			w.Header().Set("Retry-After", "2")
			w.WriteHeader(http.StatusForbidden)
		case 2:
			w.WriteHeader(http.StatusTooManyRequests)
		default:
			w.Write([]byte(`{"sha":"` + shaV5 + `"}`))
		}
	}))
	defer srv.Close()

	var slept []time.Duration
	r := NewResolver("")
	r.baseURL = srv.URL
	r.sleep = func(d time.Duration) { slept = append(slept, d) }

	sha, err := r.Resolve("actions", "checkout", "v5")
	if err != nil {
		t.Fatal(err)
	}
	if sha != shaV5 {
		t.Errorf("sha = %s", sha)
	}
	if len(slept) != 2 || slept[0] != 2*time.Second || slept[1] != 2*initialDelay {
		t.Errorf("unexpected sleeps: %v", slept)
	}
}

func TestResolverGivesUpAfterMaxAttempts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	r := NewResolver("")
	r.baseURL = srv.URL
	r.sleep = func(time.Duration) {}
	if _, err := r.Resolve("a", "b", "v1"); err == nil || !strings.Contains(err.Error(), "rate limit") {
		t.Errorf("expected rate limit error, got %v", err)
	}
}

func TestResolverPlain403NotRetried(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	r := NewResolver("")
	r.baseURL = srv.URL
	r.sleep = func(time.Duration) { t.Error("unexpected sleep") }
	if _, err := r.Resolve("a", "b", "v1"); err == nil {
		t.Error("expected error")
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1", calls)
	}
}

func TestResolverTagsPaginates(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") == "2" {
			w.Write([]byte(`[{"ref":"refs/tags/v2"}]`))
			return
		}
		w.Header().Set("Link", `<`+srv.URL+r.URL.Path+`?page=2>; rel="next"`)
		w.Write([]byte(`[{"ref":"refs/tags/v1"}]`))
	}))
	defer srv.Close()

	r := NewResolver("")
	r.baseURL = srv.URL
	tags, err := r.Tags("a", "b")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(tags, ",") != "v1,v2" {
		t.Errorf("tags = %v", tags)
	}
}

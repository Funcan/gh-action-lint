package lint

import (
	"fmt"
	"strings"
)

// DisabledChecks controls which checks are skipped during linting and fixing.
type DisabledChecks struct {
	Pins              bool // action not pinned to a SHA
	Injections        bool // script injection via ${{ }} expressions
	Permissions       bool // missing or overly broad workflow permissions
	PullRequestTarget bool // pull_request_target with untrusted code checkout
}

// ParseDisabledChecks parses a comma-separated list of check names.
// Valid names are: pins, injections, permissions, pull-request-target.
func ParseDisabledChecks(csv string) (DisabledChecks, error) {
	var d DisabledChecks
	if csv == "" {
		return d, nil
	}
	for _, name := range strings.Split(csv, ",") {
		switch strings.TrimSpace(name) {
		case "pins":
			d.Pins = true
		case "injections":
			d.Injections = true
		case "permissions":
			d.Permissions = true
		case "pull-request-target":
			d.PullRequestTarget = true
		default:
			return d, fmt.Errorf("unknown check %q (valid: pins, injections, permissions, pull-request-target)", name)
		}
	}
	return d, nil
}

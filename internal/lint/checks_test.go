package lint

import "testing"

func TestParseDisabledChecks(t *testing.T) {
	tests := []struct {
		input   string
		want    DisabledChecks
		wantErr bool
	}{
		{"", DisabledChecks{}, false},
		{"pins", DisabledChecks{Pins: true}, false},
		{"injections", DisabledChecks{Injections: true}, false},
		{"permissions", DisabledChecks{Permissions: true}, false},
		{"pull-request-target", DisabledChecks{PullRequestTarget: true}, false},
		{"pins,permissions", DisabledChecks{Pins: true, Permissions: true}, false},
		{"pins,injections,permissions", DisabledChecks{Pins: true, Injections: true, Permissions: true}, false},
		{"pins, permissions", DisabledChecks{Pins: true, Permissions: true}, false}, // spaces trimmed
		{"unknown", DisabledChecks{}, true},
		{"pins,bogus", DisabledChecks{}, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseDisabledChecks(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseDisabledChecks(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("ParseDisabledChecks(%q) = %+v, want %+v", tt.input, got, tt.want)
			}
		})
	}
}

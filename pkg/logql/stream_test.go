package logql

import (
	"strings"
	"testing"

	"github.com/prometheus/prometheus/model/labels"
)

func TestParseStreamSelector(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantMatchers int
		checkMatcher func(*testing.T, *StreamSelector)
		wantErr     bool
		errMsg      string
	}{
		{
			name:         "single label matcher",
			input:        `{job="varlogs"}`,
			wantMatchers: 1,
			checkMatcher: func(t *testing.T, sel *StreamSelector) {
				if sel.Matchers[0].Name != "job" {
					t.Errorf("matcher name = %s, want job", sel.Matchers[0].Name)
				}
				if sel.Matchers[0].Value != "varlogs" {
					t.Errorf("matcher value = %s, want varlogs", sel.Matchers[0].Value)
				}
				if sel.Matchers[0].Type != labels.MatchEqual {
					t.Errorf("matcher type = %s, want =", sel.Matchers[0].Type)
				}
			},
		},
		{
			name:         "multiple label matchers",
			input:        `{job="varlogs", level="error"}`,
			wantMatchers: 2,
			checkMatcher: func(t *testing.T, sel *StreamSelector) {
				if sel.Matchers[0].Name != "job" {
					t.Errorf("first matcher name = %s, want job", sel.Matchers[0].Name)
				}
				if sel.Matchers[1].Name != "level" {
					t.Errorf("second matcher name = %s, want level", sel.Matchers[1].Name)
				}
			},
		},
		{
			name:         "not equal matcher",
			input:        `{job="varlogs", level!="debug"}`,
			wantMatchers: 2,
			checkMatcher: func(t *testing.T, sel *StreamSelector) {
				if sel.Matchers[1].Type != labels.MatchNotEqual {
					t.Errorf("matcher type = %s, want !=", sel.Matchers[1].Type)
				}
			},
		},
		{
			name:         "regex matcher",
			input:        `{job=~"var.*"}`,
			wantMatchers: 1,
			checkMatcher: func(t *testing.T, sel *StreamSelector) {
				if sel.Matchers[0].Type != labels.MatchRegexp {
					t.Errorf("matcher type = %s, want =~", sel.Matchers[0].Type)
				}
				if sel.Matchers[0].Value != "var.*" {
					t.Errorf("matcher value = %s, want var.*", sel.Matchers[0].Value)
				}
			},
		},
		{
			name:         "negative regex matcher",
			input:        `{job="varlogs", level!~"debug.*"}`,
			wantMatchers: 2,
			checkMatcher: func(t *testing.T, sel *StreamSelector) {
				if sel.Matchers[1].Type != labels.MatchNotRegexp {
					t.Errorf("matcher type = %s, want !~", sel.Matchers[1].Type)
				}
			},
		},
		{
			name:         "mixed matcher types",
			input:        `{job="varlogs", level!="debug", service=~"api.*", host!~"test.*"}`,
			wantMatchers: 4,
			checkMatcher: func(t *testing.T, sel *StreamSelector) {
				if sel.Matchers[0].Type != labels.MatchEqual {
					t.Errorf("matcher 0 type = %s, want =", sel.Matchers[0].Type)
				}
				if sel.Matchers[1].Type != labels.MatchNotEqual {
					t.Errorf("matcher 1 type = %s, want !=", sel.Matchers[1].Type)
				}
				if sel.Matchers[2].Type != labels.MatchRegexp {
					t.Errorf("matcher 2 type = %s, want =~", sel.Matchers[2].Type)
				}
				if sel.Matchers[3].Type != labels.MatchNotRegexp {
					t.Errorf("matcher 3 type = %s, want !~", sel.Matchers[3].Type)
				}
			},
		},
		{
			name:         "whitespace handling",
			input:        `{  job  =  "varlogs"  ,  level  =  "error"  }`,
			wantMatchers: 2,
			checkMatcher: func(t *testing.T, sel *StreamSelector) {
				if len(sel.Matchers) != 2 {
					t.Errorf("expected 2 matchers, got %d", len(sel.Matchers))
				}
			},
		},
		{
			name:   "empty selector",
			input:  `{}`,
			wantErr: true,
			errMsg: "failed to parse stream selector",
		},
		{
			name:   "missing opening brace",
			input:  `job="varlogs"}`,
			wantErr: true,
			errMsg: "failed to parse stream selector",
		},
		{
			name:   "missing closing brace",
			input:  `{job="varlogs"`,
			wantErr: true,
			errMsg: "failed to parse stream selector",
		},
		{
			name:   "invalid matcher syntax",
			input:  `{job}`,
			wantErr: true,
			errMsg: "failed to parse stream selector",
		},
		{
			name:   "missing value",
			input:  `{job=}`,
			wantErr: true,
			errMsg: "failed to parse stream selector",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sel, err := ParseStreamSelector(tt.input)

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errMsg)
					return
				}
				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("error = %v, want error containing %q", err, tt.errMsg)
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if len(sel.Matchers) != tt.wantMatchers {
				t.Errorf("matchers count = %d, want %d", len(sel.Matchers), tt.wantMatchers)
				return
			}

			if tt.checkMatcher != nil {
				tt.checkMatcher(t, sel)
			}
		})
	}
}

func TestValidateStreamSelector(t *testing.T) {
	tests := []struct {
		name     string
		selector *StreamSelector
		wantErr  bool
		errMsg   string
	}{
		{
			name: "valid with equal matcher",
			selector: &StreamSelector{
				Matchers: []*labels.Matcher{
					labels.MustNewMatcher(labels.MatchEqual, "job", "varlogs"),
				},
			},
			wantErr: false,
		},
		{
			name: "valid with regex matcher",
			selector: &StreamSelector{
				Matchers: []*labels.Matcher{
					labels.MustNewMatcher(labels.MatchRegexp, "job", "var.*"),
				},
			},
			wantErr: false,
		},
		{
			name: "valid with positive and negative matchers",
			selector: &StreamSelector{
				Matchers: []*labels.Matcher{
					labels.MustNewMatcher(labels.MatchEqual, "job", "varlogs"),
					labels.MustNewMatcher(labels.MatchNotEqual, "level", "debug"),
				},
			},
			wantErr: false,
		},
		{
			name: "invalid - no matchers",
			selector: &StreamSelector{
				Matchers: []*labels.Matcher{},
			},
			wantErr: true,
			errMsg:  "stream selector must have at least one label matcher",
		},
		{
			name: "invalid - only negative matchers",
			selector: &StreamSelector{
				Matchers: []*labels.Matcher{
					labels.MustNewMatcher(labels.MatchNotEqual, "job", "test"),
					labels.MustNewMatcher(labels.MatchNotRegexp, "level", "debug.*"),
				},
			},
			wantErr: true,
			errMsg:  "stream selector must contain at least one positive matcher",
		},
		{
			name:     "invalid - nil selector",
			selector: &StreamSelector{},
			wantErr:  true,
			errMsg:   "stream selector must have at least one label matcher",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateStreamSelector(tt.selector)

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errMsg)
					return
				}
				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("error = %v, want error containing %q", err, tt.errMsg)
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestParseStreamSelectorWithContext(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		wantMatchers   int
		wantMetricName string
		wantErr        bool
	}{
		{
			name:           "no metric name",
			input:          `{job="varlogs"}`,
			wantMatchers:   1,
			wantMetricName: "",
		},
		{
			name:           "with metric name",
			input:          `metric_name{job="varlogs"}`,
			wantMatchers:   1,
			wantMetricName: "metric_name",
		},
		{
			name:           "metric name is filtered out from matchers",
			input:          `my_metric{job="varlogs", level="error"}`,
			wantMatchers:   2,
			wantMetricName: "my_metric",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sel, metricName, err := ParseStreamSelectorWithContext(tt.input)

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
					return
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if len(sel.Matchers) != tt.wantMatchers {
				t.Errorf("matchers count = %d, want %d", len(sel.Matchers), tt.wantMatchers)
			}

			if metricName != tt.wantMetricName {
				t.Errorf("metric name = %q, want %q", metricName, tt.wantMetricName)
			}

			// Verify __name__ matcher is not in the matchers list
			for _, m := range sel.Matchers {
				if m.Name == labels.MetricName {
					t.Errorf("found __name__ matcher in matchers list, should be filtered out")
				}
			}
		})
	}
}

func TestStreamSelector_String(t *testing.T) {
	tests := []struct {
		name     string
		selector *StreamSelector
		want     string
	}{
		{
			name: "empty selector",
			selector: &StreamSelector{
				Matchers: []*labels.Matcher{},
			},
			want: "{}",
		},
		{
			name: "single matcher",
			selector: &StreamSelector{
				Matchers: []*labels.Matcher{
					labels.MustNewMatcher(labels.MatchEqual, "job", "varlogs"),
				},
			},
			want: `{job="varlogs"}`,
		},
		{
			name: "multiple matchers",
			selector: &StreamSelector{
				Matchers: []*labels.Matcher{
					labels.MustNewMatcher(labels.MatchEqual, "job", "varlogs"),
					labels.MustNewMatcher(labels.MatchNotEqual, "level", "debug"),
				},
			},
			want: `{job="varlogs", level!="debug"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.selector.String()
			if got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

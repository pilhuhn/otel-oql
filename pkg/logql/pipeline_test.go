package logql

import (
	"strings"
	"testing"
)

func TestParsePipeline(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantStages int
		checkStage func(*testing.T, []PipelineStage)
		wantErr    bool
		errMsg     string
	}{
		{
			name:       "empty pipeline",
			input:      "",
			wantStages: 0,
		},
		{
			name:       "whitespace only",
			input:      "   ",
			wantStages: 0,
		},
		{
			name:       "line filter contains",
			input:      `|= "error"`,
			wantStages: 1,
			checkStage: func(t *testing.T, stages []PipelineStage) {
				lf, ok := stages[0].(*LineFilter)
				if !ok {
					t.Errorf("expected LineFilter, got %T", stages[0])
					return
				}
				if lf.Operator != "|=" {
					t.Errorf("operator = %s, want |=", lf.Operator)
				}
				if lf.Value != "error" {
					t.Errorf("value = %s, want error", lf.Value)
				}
			},
		},
		{
			name:       "line filter not contains",
			input:      `!= "debug"`,
			wantStages: 1,
			checkStage: func(t *testing.T, stages []PipelineStage) {
				lf, ok := stages[0].(*LineFilter)
				if !ok {
					t.Errorf("expected LineFilter, got %T", stages[0])
					return
				}
				if lf.Operator != "!=" {
					t.Errorf("operator = %s, want !=", lf.Operator)
				}
				if lf.Value != "debug" {
					t.Errorf("value = %s, want debug", lf.Value)
				}
			},
		},
		{
			name:       "line filter regex match",
			input:      `|~ "error|fail"`,
			wantStages: 1,
			checkStage: func(t *testing.T, stages []PipelineStage) {
				lf, ok := stages[0].(*LineFilter)
				if !ok {
					t.Errorf("expected LineFilter, got %T", stages[0])
					return
				}
				if lf.Operator != "|~" {
					t.Errorf("operator = %s, want |~", lf.Operator)
				}
				if lf.Value != "error|fail" {
					t.Errorf("value = %s, want error|fail", lf.Value)
				}
			},
		},
		{
			name:       "line filter regex not match",
			input:      `!~ "debug|trace"`,
			wantStages: 1,
			checkStage: func(t *testing.T, stages []PipelineStage) {
				lf, ok := stages[0].(*LineFilter)
				if !ok {
					t.Errorf("expected LineFilter, got %T", stages[0])
					return
				}
				if lf.Operator != "!~" {
					t.Errorf("operator = %s, want !~", lf.Operator)
				}
				if lf.Value != "debug|trace" {
					t.Errorf("value = %s, want debug|trace", lf.Value)
				}
			},
		},
		{
			name:       "label parser json",
			input:      `| json`,
			wantStages: 1,
			checkStage: func(t *testing.T, stages []PipelineStage) {
				lp, ok := stages[0].(*LabelParser)
				if !ok {
					t.Errorf("expected LabelParser, got %T", stages[0])
					return
				}
				if lp.Type != "json" {
					t.Errorf("type = %s, want json", lp.Type)
				}
			},
		},
		{
			name:       "label parser logfmt",
			input:      `| logfmt`,
			wantStages: 1,
			checkStage: func(t *testing.T, stages []PipelineStage) {
				lp, ok := stages[0].(*LabelParser)
				if !ok {
					t.Errorf("expected LabelParser, got %T", stages[0])
					return
				}
				if lp.Type != "logfmt" {
					t.Errorf("type = %s, want logfmt", lp.Type)
				}
			},
		},
		{
			name:       "label parser pattern",
			input:      `| pattern`,
			wantStages: 1,
			checkStage: func(t *testing.T, stages []PipelineStage) {
				lp, ok := stages[0].(*LabelParser)
				if !ok {
					t.Errorf("expected LabelParser, got %T", stages[0])
					return
				}
				if lp.Type != "pattern" {
					t.Errorf("type = %s, want pattern", lp.Type)
				}
			},
		},
		{
			name:       "label parser regexp",
			input:      `| regexp`,
			wantStages: 1,
			checkStage: func(t *testing.T, stages []PipelineStage) {
				lp, ok := stages[0].(*LabelParser)
				if !ok {
					t.Errorf("expected LabelParser, got %T", stages[0])
					return
				}
				if lp.Type != "regexp" {
					t.Errorf("type = %s, want regexp", lp.Type)
				}
			},
		},
		{
			name:       "multiple stages",
			input:      `|= "error" != "timeout"`,
			wantStages: 2,
			checkStage: func(t *testing.T, stages []PipelineStage) {
				lf1, ok := stages[0].(*LineFilter)
				if !ok {
					t.Errorf("stage 0: expected LineFilter, got %T", stages[0])
					return
				}
				if lf1.Operator != "|=" || lf1.Value != "error" {
					t.Errorf("stage 0: operator=%s value=%s, want |= error", lf1.Operator, lf1.Value)
				}

				lf2, ok := stages[1].(*LineFilter)
				if !ok {
					t.Errorf("stage 1: expected LineFilter, got %T", stages[1])
					return
				}
				if lf2.Operator != "!=" || lf2.Value != "timeout" {
					t.Errorf("stage 1: operator=%s value=%s, want != timeout", lf2.Operator, lf2.Value)
				}
			},
		},
		{
			name:       "line filter then parser",
			input:      `|= "error" | json`,
			wantStages: 2,
			checkStage: func(t *testing.T, stages []PipelineStage) {
				if _, ok := stages[0].(*LineFilter); !ok {
					t.Errorf("stage 0: expected LineFilter, got %T", stages[0])
				}
				if _, ok := stages[1].(*LabelParser); !ok {
					t.Errorf("stage 1: expected LabelParser, got %T", stages[1])
				}
			},
		},
		{
			name:       "parser then line filter",
			input:      `| json |= "error"`,
			wantStages: 2,
			checkStage: func(t *testing.T, stages []PipelineStage) {
				if _, ok := stages[0].(*LabelParser); !ok {
					t.Errorf("stage 0: expected LabelParser, got %T", stages[0])
				}
				if _, ok := stages[1].(*LineFilter); !ok {
					t.Errorf("stage 1: expected LineFilter, got %T", stages[1])
				}
			},
		},
		{
			name:       "complex pipeline",
			input:      `|= "error" | json != "timeout" |~ "fail.*"`,
			wantStages: 4,
			checkStage: func(t *testing.T, stages []PipelineStage) {
				if len(stages) != 4 {
					t.Errorf("expected 4 stages, got %d", len(stages))
				}
			},
		},
		{
			name:       "whitespace handling",
			input:      `  |=  "error"   |   json  `,
			wantStages: 2,
		},
		{
			name:       "escaped quotes in value",
			input:      `|= "error \"message\""`,
			wantStages: 1,
			checkStage: func(t *testing.T, stages []PipelineStage) {
				lf, ok := stages[0].(*LineFilter)
				if !ok {
					t.Errorf("expected LineFilter, got %T", stages[0])
					return
				}
				if lf.Value != `error "message"` {
					t.Errorf("value = %s, want error \"message\"", lf.Value)
				}
			},
		},
		{
			name:       "backslash in value",
			input:      `|= "path\\to\\file"`,
			wantStages: 1,
			checkStage: func(t *testing.T, stages []PipelineStage) {
				lf, ok := stages[0].(*LineFilter)
				if !ok {
					t.Errorf("expected LineFilter, got %T", stages[0])
					return
				}
				if lf.Value != `path\to\file` {
					t.Errorf("value = %s, want path\\to\\file", lf.Value)
				}
			},
		},
		{
			name:    "missing pipe",
			input:   `= "error"`,
			wantErr: true,
			errMsg:  "pipeline stage must start with |",
		},
		{
			name:    "unknown operator",
			input:   `|> "error"`,
			wantErr: true,
			errMsg:  "unknown pipeline stage",
		},
		{
			name:    "unclosed string",
			input:   `|= "error`,
			wantErr: true,
			errMsg:  "unknown pipeline stage",
		},
		{
			name:    "missing value",
			input:   `|=`,
			wantErr: true,
			errMsg:  "unknown pipeline stage",
		},
		{
			name:    "invalid parser name",
			input:   `| unknown_parser`,
			wantErr: true,
			errMsg:  "unknown pipeline stage",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stages, err := ParsePipeline(tt.input)

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

			if len(stages) != tt.wantStages {
				t.Errorf("stages count = %d, want %d", len(stages), tt.wantStages)
				return
			}

			if tt.checkStage != nil {
				tt.checkStage(t, stages)
			}
		})
	}
}

func TestSplitQueryParts(t *testing.T) {
	tests := []struct {
		name             string
		query            string
		wantSelector     string
		wantPipeline     string
		wantErr          bool
		errMsg           string
	}{
		{
			name:         "selector only",
			query:        `{job="varlogs"}`,
			wantSelector: `{job="varlogs"}`,
			wantPipeline: "",
		},
		{
			name:         "selector with pipeline",
			query:        `{job="varlogs"} |= "error"`,
			wantSelector: `{job="varlogs"}`,
			wantPipeline: `|= "error"`,
		},
		{
			name:         "selector with multiple pipeline stages",
			query:        `{job="varlogs"} |= "error" | json`,
			wantSelector: `{job="varlogs"}`,
			wantPipeline: `|= "error" | json`,
		},
		{
			name:         "nested braces in selector",
			query:        `{job="varlogs", label="{nested}"}`,
			wantSelector: `{job="varlogs", label="{nested}"}`,
			wantPipeline: "",
		},
		{
			name:         "nested braces with pipeline",
			query:        `{job="varlogs", label="{nested}"} |= "error"`,
			wantSelector: `{job="varlogs", label="{nested}"}`,
			wantPipeline: `|= "error"`,
		},
		{
			name:         "whitespace handling",
			query:        `  {job="varlogs"}   |= "error"  `,
			wantSelector: `{job="varlogs"}`,
			wantPipeline: `|= "error"`,
		},
		{
			name:    "missing opening brace",
			query:   `job="varlogs"}`,
			wantErr: true,
			errMsg:  "query must start with stream selector",
		},
		{
			name:    "unclosed selector",
			query:   `{job="varlogs"`,
			wantErr: true,
			errMsg:  "unclosed stream selector",
		},
		{
			name:    "empty query",
			query:   "",
			wantErr: true,
			errMsg:  "query must start with stream selector",
		},
		{
			name:    "only whitespace",
			query:   "   ",
			wantErr: true,
			errMsg:  "query must start with stream selector",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			selector, pipeline, err := SplitQueryParts(tt.query)

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

			if selector != tt.wantSelector {
				t.Errorf("selector = %q, want %q", selector, tt.wantSelector)
			}

			if pipeline != tt.wantPipeline {
				t.Errorf("pipeline = %q, want %q", pipeline, tt.wantPipeline)
			}
		})
	}
}

func TestParseStringLiteral(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		wantValue     string
		wantConsumed  int
	}{
		{
			name:         "simple string",
			input:        `"hello"`,
			wantValue:    "hello",
			wantConsumed: 7,
		},
		{
			name:         "empty string",
			input:        `""`,
			wantValue:    "",
			wantConsumed: 2,
		},
		{
			name:         "string with spaces",
			input:        `"hello world"`,
			wantValue:    "hello world",
			wantConsumed: 13,
		},
		{
			name:         "escaped quotes",
			input:        `"hello \"world\""`,
			wantValue:    `hello "world"`,
			wantConsumed: 17, // Length of input string including quotes
		},
		{
			name:         "escaped backslash",
			input:        `"path\\to\\file"`,
			wantValue:    `path\to\file`,
			wantConsumed: 16,
		},
		{
			name:         "string with trailing content",
			input:        `"hello" world`,
			wantValue:    "hello",
			wantConsumed: 7,
		},
		{
			name:         "string with leading whitespace",
			input:        `  "hello"`,
			wantValue:    "hello",
			wantConsumed: 7,
		},
		{
			name:         "no opening quote",
			input:        `hello"`,
			wantValue:    "",
			wantConsumed: 0,
		},
		{
			name:         "no closing quote",
			input:        `"hello`,
			wantValue:    "",
			wantConsumed: 0,
		},
		{
			name:         "empty input",
			input:        "",
			wantValue:    "",
			wantConsumed: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value, consumed := parseStringLiteral(tt.input)

			if value != tt.wantValue {
				t.Errorf("value = %q, want %q", value, tt.wantValue)
			}

			if consumed != tt.wantConsumed {
				t.Errorf("consumed = %d, want %d", consumed, tt.wantConsumed)
			}
		})
	}
}

func TestParsePipeline_LabelManipulation(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantStages int
		wantOp     string
		wantLabels []string
	}{
		{
			name:       "drop single label",
			input:      "| drop __error__",
			wantStages: 1,
			wantOp:     "drop",
			wantLabels: []string{"__error__"},
		},
		{
			name:       "drop multiple labels",
			input:      "| drop label1, label2, label3",
			wantStages: 1,
			wantOp:     "drop",
			wantLabels: []string{"label1", "label2", "label3"},
		},
		{
			name:       "keep labels",
			input:      "| keep level, service",
			wantStages: 1,
			wantOp:     "keep",
			wantLabels: []string{"level", "service"},
		},
		{
			name:       "drop with line filter",
			input:      "|= \"error\" | drop __error__",
			wantStages: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stages, err := ParsePipeline(tt.input)
			if err != nil {
				t.Fatalf("ParsePipeline() error = %v", err)
			}

			if len(stages) != tt.wantStages {
				t.Errorf("got %d stages, want %d", len(stages), tt.wantStages)
			}

			if tt.wantOp != "" {
				// Check the label manipulation stage
				var found bool
				for _, stage := range stages {
					if lm, ok := stage.(*LabelManipulation); ok {
						found = true
						if lm.Operation != tt.wantOp {
							t.Errorf("got operation %q, want %q", lm.Operation, tt.wantOp)
						}
						if len(lm.Labels) != len(tt.wantLabels) {
							t.Errorf("got %d labels, want %d", len(lm.Labels), len(tt.wantLabels))
						}
						for i, label := range lm.Labels {
							if i < len(tt.wantLabels) && label != tt.wantLabels[i] {
								t.Errorf("label %d: got %q, want %q", i, label, tt.wantLabels[i])
							}
						}
					}
				}
				if !found {
					t.Error("expected LabelManipulation stage, but didn't find one")
				}
			}
		})
	}
}

func TestParseStringLiteral_Backticks(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantValue string
		wantLen   int
	}{
		{
			name:      "double quotes",
			input:     `"hello"`,
			wantValue: "hello",
			wantLen:   7,
		},
		{
			name:      "backticks",
			input:     "`hello`",
			wantValue: "hello",
			wantLen:   7,
		},
		{
			name:      "backticks with special chars",
			input:     "`replicator`",
			wantValue: "replicator",
			wantLen:   12,
		},
		{
			name:      "double quotes with escapes",
			input:     `"hello \"world\""`,
			wantValue: `hello "world"`,
			wantLen:   17,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value, consumed := parseStringLiteral(tt.input)
			if value != tt.wantValue {
				t.Errorf("got value %q, want %q", value, tt.wantValue)
			}
			if consumed != tt.wantLen {
				t.Errorf("got consumed %d, want %d", consumed, tt.wantLen)
			}
		})
	}
}

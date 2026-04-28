package querylangs

import (
	"fmt"
	"testing"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"
)

// TestLogQLStreamSelectorReuse tests if we can reuse PromQL parser for LogQL stream selectors
func TestLogQLStreamSelectorReuse(t *testing.T) {
	// LogQL stream selectors are identical to PromQL label selectors
	// Note: PromQL requires at least one positive matcher (=, =~)
	logQLStreamSelectors := []string{
		`{job="varlogs"}`,
		`{job="varlogs", level="error"}`,
		`{job=~"var.*"}`,
		`{job="varlogs", level!="debug"}`, // Must have at least one positive matcher
		`{job=~"var.*", level!~"test.*"}`, // Must have at least one positive matcher
	}

	t.Log("Testing LogQL Stream Selector Reuse:")
	t.Log("=====================================")

	for _, query := range logQLStreamSelectors {
		t.Run(query, func(t *testing.T) {
			p := parser.NewParser(parser.Options{})
			expr, err := p.ParseExpr(query)
			if err != nil {
				t.Fatalf("Failed to parse: %v", err)
			}

			// Check if it parsed as a VectorSelector
			vs, ok := expr.(*parser.VectorSelector)
			if !ok {
				t.Fatalf("Expected VectorSelector, got %T", expr)
			}

			t.Logf("✓ Parsed successfully as VectorSelector")
			t.Logf("  Label matchers:")
			for _, matcher := range vs.LabelMatchers {
				t.Logf("    %s %s %s", matcher.Name, matcher.Type, matcher.Value)
			}

			// This means we can reuse PromQL parser for LogQL stream selectors!
			t.Log("  → REUSE OPPORTUNITY: LogQL stream selectors can use PromQL parser")
		})
	}
}

// TestLogQLAggregationReuse tests if LogQL aggregations can reuse PromQL parser
func TestLogQLAggregationReuse(t *testing.T) {
	// LogQL metric aggregations that look like PromQL
	logQLAggregations := []string{
		`count_over_time({job="varlogs"}[5m])`,
		`rate({job="varlogs"}[5m])`,
		`sum by (level) (count_over_time({job="varlogs"}[1h]))`,
	}

	t.Log("\nTesting LogQL Aggregation Reuse:")
	t.Log("================================")

	for _, query := range logQLAggregations {
		t.Run(query, func(t *testing.T) {
			p := parser.NewParser(parser.Options{})
			expr, err := p.ParseExpr(query)
			if err != nil {
				t.Fatalf("Failed to parse: %v", err)
			}

			t.Logf("✓ Parsed as: %T", expr)

			switch e := expr.(type) {
			case *parser.Call:
				t.Logf("  Function: %s", e.Func.Name)
				t.Log("  → REUSE OPPORTUNITY: LogQL metric queries can use PromQL parser")
			case *parser.AggregateExpr:
				t.Logf("  Aggregation: %s", e.Op)
				t.Log("  → REUSE OPPORTUNITY: LogQL aggregations can use PromQL parser")
			}
		})
	}
}

// TestSharedComponents analyzes what components can be shared
func TestSharedComponents(t *testing.T) {
	t.Log("\nShared Component Analysis:")
	t.Log("==========================\n")

	components := []struct {
		component   string
		promql      string
		logql       string
		traceql     string
		shareable   bool
		shareWith   string
		description string
	}{
		{
			component:   "Label Selector Parser",
			promql:      "{label=\"value\"}",
			logql:       "{label=\"value\"}",
			traceql:     "{field=\"value\"}",
			shareable:   true,
			shareWith:   "All three (with minor TraceQL extensions)",
			description: "Curly brace selector parsing",
		},
		{
			component:   "Label Matcher Types",
			promql:      "=, !=, =~, !~",
			logql:       "=, !=, =~, !~",
			traceql:     "=, !=, >, <, >=, <=",
			shareable:   true,
			shareWith:   "PromQL/LogQL (TraceQL needs extensions)",
			description: "Operator parsing for matchers",
		},
		{
			component:   "Time Range Parser",
			promql:      "[5m], [1h]",
			logql:       "[5m], [1h]",
			traceql:     "N/A",
			shareable:   true,
			shareWith:   "PromQL/LogQL",
			description: "Square bracket time duration parsing",
		},
		{
			component:   "Aggregation Functions",
			promql:      "sum, avg, count, min, max",
			logql:       "sum, avg, count, min, max",
			traceql:     "count, avg, min, max",
			shareable:   true,
			shareWith:   "All three",
			description: "Aggregation function names",
		},
		{
			component:   "Grouping Syntax",
			promql:      "by (label1, label2)",
			logql:       "by (label1, label2)",
			traceql:     "Different approach",
			shareable:   true,
			shareWith:   "PromQL/LogQL",
			description: "'by' keyword and label list",
		},
		{
			component:   "Pipe Operators",
			promql:      "N/A",
			logql:       "|=, !=, |~, !~, | json, | logfmt",
			traceql:     "N/A",
			shareable:   false,
			shareWith:   "LogQL only",
			description: "Log-specific pipeline operators",
		},
		{
			component:   "Dot Notation",
			promql:      "N/A",
			logql:       "N/A",
			traceql:     ".attribute, resource.attribute",
			shareable:   false,
			shareWith:   "TraceQL only",
			description: "Span/resource attribute syntax",
		},
	}

	for _, c := range components {
		t.Logf("Component: %s", c.component)
		t.Logf("  Description: %s", c.description)
		t.Logf("  PromQL:  %s", c.promql)
		t.Logf("  LogQL:   %s", c.logql)
		t.Logf("  TraceQL: %s", c.traceql)
		if c.shareable {
			t.Logf("  ✓ SHAREABLE: %s", c.shareWith)
		} else {
			t.Logf("  ✗ NOT SHAREABLE: %s", c.shareWith)
		}
		t.Log("")
	}
}

// TestProposedArchitecture proposes a shared architecture
func TestProposedArchitecture(t *testing.T) {
	t.Log("\nProposed Shared Architecture:")
	t.Log("=============================\n")

	architecture := `
Shared Components (pkg/querylangs/):
├─ common/
│  ├─ selector_parser.go      # Parse {label="value"} selectors
│  ├─ matcher_parser.go        # Parse =, !=, =~, !~ matchers
│  ├─ timerange_parser.go      # Parse [5m] time ranges
│  ├─ aggregation.go           # Common aggregation logic
│  └─ types.go                 # Shared AST node types
│
Language-Specific (existing structure):
├─ promql/
│  ├─ translator.go            # Uses Prometheus parser + our common translator
│  └─ ...
│
├─ logql/
│  ├─ parser.go                # Custom parser for pipes + uses common selectors
│  ├─ stream_selector.go       # Wraps common selector parser
│  ├─ pipeline_parser.go       # Parse |=, |~, | json, etc (LogQL-specific)
│  ├─ translator.go            # LogQL AST → SQL
│  └─ ...
│
└─ traceql/
   ├─ parser.go                # Custom parser with extended matchers
   ├─ selector_parser.go       # Based on common but adds >, <, etc
   ├─ dot_notation.go          # Parse .attr, resource.attr (TraceQL-specific)
   ├─ translator.go            # TraceQL AST → SQL
   └─ ...
`

	t.Log(architecture)

	t.Log("\nReuse Strategy:")
	t.Log("===============")
	t.Log("1. PromQL: Already done - uses Prometheus parser directly")
	t.Log("2. LogQL:  Stream selectors → reuse Prometheus parser")
	t.Log("           Pipe operators → custom parsing needed")
	t.Log("           Aggregations → can reuse Prometheus parser")
	t.Log("3. TraceQL: Selector syntax → extend common parser")
	t.Log("            Dot notation → custom parsing needed")
	t.Log("")
}

// TestLabelMatcherCompatibility tests label matcher compatibility
func TestLabelMatcherCompatibility(t *testing.T) {
	t.Log("\nLabel Matcher Compatibility:")
	t.Log("============================\n")

	matchers := []struct {
		syntax  string
		promql  bool
		logql   bool
		traceql bool
	}{
		{"label = \"value\"", true, true, true},
		{"label != \"value\"", true, true, true},
		{"label =~ \"regex\"", true, true, false},
		{"label !~ \"regex\"", true, true, false},
		{"field > 100", false, false, true},
		{"field < 100", false, false, true},
		{"field >= 100", false, false, true},
		{"field <= 100", false, false, true},
	}

	for _, m := range matchers {
		langs := []string{}
		if m.promql {
			langs = append(langs, "PromQL")
		}
		if m.logql {
			langs = append(langs, "LogQL")
		}
		if m.traceql {
			langs = append(langs, "TraceQL")
		}

		t.Logf("%-20s → Supported by: %v", m.syntax, langs)

		// Test if Prometheus parser handles it
		testQuery := fmt.Sprintf("{test%s}", m.syntax)
		p := parser.NewParser(parser.Options{})
		_, err := p.ParseExpr(testQuery)
		if err == nil {
			t.Logf("  ✓ Prometheus parser accepts this")
		} else {
			t.Logf("  ✗ Prometheus parser rejects: %v", err)
		}
	}
}

// TestMatcherTypeMapping shows how to map between parser types
func TestMatcherTypeMapping(t *testing.T) {
	t.Log("\nMatcher Type Mapping:")
	t.Log("=====================\n")

	query := `{job="api", level=~"error|warn", status!="200"}`
	p := parser.NewParser(parser.Options{})
	expr, err := p.ParseExpr(query)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	vs := expr.(*parser.VectorSelector)

	t.Logf("Query: %s\n", query)
	t.Log("Prometheus Label Matchers:")
	for _, m := range vs.LabelMatchers {
		sqlOp := ""
		switch m.Type {
		case labels.MatchEqual:
			sqlOp = "="
		case labels.MatchNotEqual:
			sqlOp = "<>"
		case labels.MatchRegexp:
			sqlOp = "REGEXP_LIKE"
		case labels.MatchNotRegexp:
			sqlOp = "NOT REGEXP_LIKE"
		}

		t.Logf("  %s %v %q → SQL: %s %s '%s'",
			m.Name, m.Type, m.Value,
			m.Name, sqlOp, m.Value)
	}

	t.Log("\n→ This mapping works for both PromQL and LogQL!")
}

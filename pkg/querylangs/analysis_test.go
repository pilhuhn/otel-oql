package querylangs

import (
	"testing"

	"github.com/prometheus/prometheus/promql/parser"
)

// TestPromQLParserCapabilities explores what the Prometheus parser can handle
// This helps us understand if we can reuse it for LogQL and TraceQL
func TestPromQLParserCapabilities(t *testing.T) {
	tests := []struct {
		name        string
		query       string
		lang        string
		parseable   bool
		description string
	}{
		// PromQL examples
		{
			name:        "PromQL - instant vector",
			query:       `http_requests_total{job="api"}`,
			lang:        "promql",
			parseable:   true,
			description: "Standard PromQL instant vector selector",
		},
		{
			name:        "PromQL - range vector",
			query:       `http_requests_total[5m]`,
			lang:        "promql",
			parseable:   true,
			description: "PromQL range vector with time window",
		},
		{
			name:        "PromQL - aggregation",
			query:       `sum by (job) (http_requests_total)`,
			lang:        "promql",
			parseable:   true,
			description: "PromQL aggregation with grouping",
		},

		// LogQL-like queries - do they parse as PromQL?
		{
			name:        "LogQL - stream selector only",
			query:       `{job="varlogs"}`,
			lang:        "logql",
			parseable:   true, // PromQL allows this!
			description: "LogQL stream selector looks like PromQL label matcher",
		},
		{
			name:        "LogQL - with line filter",
			query:       `{job="varlogs"} |= "error"`,
			lang:        "logql",
			parseable:   false, // PromQL parser won't understand |=
			description: "LogQL pipe operators are LogQL-specific",
		},
		{
			name:        "LogQL - count_over_time",
			query:       `count_over_time({job="varlogs"}[5m])`,
			lang:        "logql",
			parseable:   true, // This is actually valid PromQL syntax too!
			description: "LogQL count_over_time is compatible with PromQL",
		},

		// TraceQL-like queries
		{
			name:        "TraceQL - span filter",
			query:       `{duration > 1s}`,
			lang:        "traceql",
			parseable:   false, // PromQL requires label matchers to be =, !=, =~, !~
			description: "TraceQL uses comparison operators in selectors",
		},
		{
			name:        "TraceQL - span attribute",
			query:       `{.http.status_code = 500}`,
			lang:        "traceql",
			parseable:   false, // Dot notation not valid in PromQL
			description: "TraceQL uses dot prefix for span attributes",
		},
		{
			name:        "TraceQL - resource attribute",
			query:       `{resource.service.name = "api"}`,
			lang:        "traceql",
			parseable:   false, // 'resource' is not a valid metric name with dots
			description: "TraceQL uses resource prefix for resource attributes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := parser.NewParser(parser.Options{})
			_, err := p.ParseExpr(tt.query)
			parsed := (err == nil)

			t.Logf("Query: %s", tt.query)
			t.Logf("Language: %s", tt.lang)
			t.Logf("Description: %s", tt.description)
			t.Logf("Parseable by PromQL parser: %v", parsed)
			t.Logf("Expected parseable: %v", tt.parseable)

			if parsed != tt.parseable {
				if !tt.parseable && parsed {
					t.Logf("INTERESTING: PromQL parser unexpectedly accepted %s query", tt.lang)
				}
			}
		})
	}
}

// TestCommonSyntaxPatterns identifies shared syntax patterns
func TestCommonSyntaxPatterns(t *testing.T) {
	patterns := []struct {
		pattern     string
		promql      bool
		logql       bool
		traceql     bool
		description string
	}{
		{
			pattern:     `{label="value"}`,
			promql:      true,
			logql:       true,
			traceql:     true,
			description: "Label/attribute selector with equals",
		},
		{
			pattern:     `{label=~"regex"}`,
			promql:      true,
			logql:       true,
			traceql:     false, // TraceQL doesn't use =~ for regex
			description: "Regex label matcher",
		},
		{
			pattern:     `[5m]`,
			promql:      true,
			logql:       true,
			traceql:     false, // TraceQL doesn't use time ranges this way
			description: "Time range/window syntax",
		},
		{
			pattern:     `sum by (label)`,
			promql:      true,
			logql:       true,
			traceql:     false, // TraceQL has different aggregation syntax
			description: "Aggregation with grouping",
		},
		{
			pattern:     `|= "text"`,
			promql:      false,
			logql:       true,
			traceql:     false,
			description: "Pipe operator for filtering (LogQL-specific)",
		},
		{
			pattern:     `{.attribute = value}`,
			promql:      false,
			logql:       false,
			traceql:     true,
			description: "Dot-prefixed span attributes (TraceQL-specific)",
		},
		{
			pattern:     `> comparison`,
			promql:      true, // metric > 100
			logql:       false, // Not in stream selector
			traceql:     true, // {duration > 1s}
			description: "Comparison operators in selector",
		},
	}

	t.Log("Common Syntax Analysis:")
	t.Log("========================")

	for _, p := range patterns {
		t.Logf("\nPattern: %s", p.pattern)
		t.Logf("Description: %s", p.description)
		t.Logf("  PromQL:  %v", p.promql)
		t.Logf("  LogQL:   %v", p.logql)
		t.Logf("  TraceQL: %v", p.traceql)

		shared := 0
		if p.promql {
			shared++
		}
		if p.logql {
			shared++
		}
		if p.traceql {
			shared++
		}

		if shared >= 2 {
			t.Logf("  → SHARED by %d languages (potential for code reuse)", shared)
		}
	}
}

// TestQueryStructureComparison compares the structure of queries
func TestQueryStructureComparison(t *testing.T) {
	comparison := []struct {
		aspect      string
		promql      string
		logql       string
		traceql     string
		commonality string
	}{
		{
			aspect:      "Selector syntax",
			promql:      "{label=value}",
			logql:       "{label=value}",
			traceql:     "{field=value}",
			commonality: "All use curly braces for selectors",
		},
		{
			aspect:      "Time ranges",
			promql:      "[5m]",
			logql:       "[5m]",
			traceql:     "N/A (different approach)",
			commonality: "PromQL and LogQL share time range syntax",
		},
		{
			aspect:      "Aggregation",
			promql:      "sum(metric)",
			logql:       "sum(log_query)",
			traceql:     "count() or avg()",
			commonality: "Similar function call syntax but different semantics",
		},
		{
			aspect:      "Filtering",
			promql:      "{label=~\"regex\"}",
			logql:       "|= \"text\" or |~ \"regex\"",
			traceql:     "{field > value}",
			commonality: "Different approaches - shared regex support between PromQL/LogQL",
		},
		{
			aspect:      "Grouping",
			promql:      "by (label)",
			logql:       "by (label)",
			traceql:     "Different approach",
			commonality: "PromQL and LogQL share grouping syntax",
		},
	}

	t.Log("\nQuery Structure Comparison:")
	t.Log("===========================")

	for _, c := range comparison {
		t.Logf("\n%s:", c.aspect)
		t.Logf("  PromQL:      %s", c.promql)
		t.Logf("  LogQL:       %s", c.logql)
		t.Logf("  TraceQL:     %s", c.traceql)
		t.Logf("  Commonality: %s", c.commonality)
	}
}

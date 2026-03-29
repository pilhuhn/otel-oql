package logql

import (
	"fmt"
	"strings"
)

// ParsePipeline parses LogQL pipeline stages
// Examples: |= "error", != "debug", |~ "error|warn", | json, | logfmt
func ParsePipeline(input string) ([]PipelineStage, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil, nil
	}

	stages := make([]PipelineStage, 0)
	remaining := input

	for len(remaining) > 0 {
		remaining = strings.TrimSpace(remaining)

		// Check if it starts with | (for |=, |~, | json, etc.)
		// or with ! (for !=, !~)
		var stageInput string
		if strings.HasPrefix(remaining, "|") {
			// Remove the |
			stageInput = strings.TrimSpace(remaining[1:])
		} else if strings.HasPrefix(remaining, "!") {
			// != and !~ don't have leading |
			stageInput = remaining
		} else {
			return nil, fmt.Errorf("pipeline stage must start with | or !, got: %s", remaining)
		}

		// Try to parse different pipeline stages
		stage, consumed, err := parseNextStage(stageInput)
		if err != nil {
			return nil, err
		}

		stages = append(stages, stage)

		// Calculate how much we consumed from the original remaining string
		if strings.HasPrefix(remaining, "|") {
			remaining = strings.TrimSpace(stageInput[consumed:])
		} else {
			remaining = strings.TrimSpace(remaining[consumed:])
		}
	}

	return stages, nil
}

// parseNextStage parses the next pipeline stage and returns it along with
// the number of characters consumed
func parseNextStage(input string) (PipelineStage, int, error) {
	// Try line filter first (|=, !=, |~, !~)
	if stage, consumed := tryParseLineFilter(input); stage != nil {
		return stage, consumed, nil
	}

	// Try label parser (json, logfmt, etc.)
	if stage, consumed := tryParseLabelParser(input); stage != nil {
		return stage, consumed, nil
	}

	// Try label manipulation (drop, keep)
	if stage, consumed := tryParseLabelManipulation(input); stage != nil {
		return stage, consumed, nil
	}

	return nil, 0, fmt.Errorf("unknown pipeline stage: %s", input)
}

// tryParseLineFilter tries to parse a line filter: = "text", != "text", ~ "regex", !~ "regex"
func tryParseLineFilter(input string) (*LineFilter, int) {
	input = strings.TrimSpace(input)

	var operator string
	var rest string

	// Check for operators
	if strings.HasPrefix(input, "!~") {
		operator = "!~"
		rest = strings.TrimSpace(input[2:])
	} else if strings.HasPrefix(input, "!=") {
		operator = "!="
		rest = strings.TrimSpace(input[2:])
	} else if strings.HasPrefix(input, "=") {
		operator = "|="
		rest = strings.TrimSpace(input[1:])
	} else if strings.HasPrefix(input, "~") {
		operator = "|~"
		rest = strings.TrimSpace(input[1:])
	} else {
		return nil, 0
	}

	// Parse the string value
	value, consumed := parseStringLiteral(rest)
	if value == "" {
		return nil, 0
	}

	return &LineFilter{
		Operator: operator,
		Value:    value,
	}, len(input) - len(rest) + consumed
}

// tryParseLabelParser tries to parse a label parser: json, logfmt, pattern, regexp
func tryParseLabelParser(input string) (*LabelParser, int) {
	input = strings.TrimSpace(input)

	// Check for known parsers
	parsers := []string{"json", "logfmt", "pattern", "regexp"}
	for _, p := range parsers {
		if strings.HasPrefix(input, p) {
			// Check if it's followed by a space, |, or end of string
			if len(input) == len(p) || input[len(p)] == ' ' || input[len(p)] == '|' {
				return &LabelParser{
					Type:   p,
					Params: "", // TODO: Parse parameters if needed
				}, len(p)
			}
		}
	}

	return nil, 0
}

// parseStringLiteral parses a quoted string and returns the value and characters consumed
// Supports both double quotes ("text") and backticks (`text`)
func parseStringLiteral(input string) (string, int) {
	input = strings.TrimSpace(input)

	if len(input) == 0 {
		return "", 0
	}

	// Determine quote type
	var quote byte
	if input[0] == '"' {
		quote = '"'
	} else if input[0] == '`' {
		quote = '`'
	} else {
		return "", 0
	}

	// Find the closing quote (handle escapes for double quotes)
	escaped := false
	for i := 1; i < len(input); i++ {
		if quote == '"' && escaped {
			escaped = false
			continue
		}

		if quote == '"' && input[i] == '\\' {
			escaped = true
			continue
		}

		if input[i] == quote {
			// Found the closing quote
			value := input[1:i]
			// Unescape the string (only for double quotes)
			if quote == '"' {
				value = strings.ReplaceAll(value, `\"`, `"`)
				value = strings.ReplaceAll(value, `\\`, `\`)
			}
			return value, i + 1
		}
	}

	// No closing quote found
	return "", 0
}

// SplitQueryParts splits a LogQL query into stream selector and pipeline parts
// Example: {job="varlogs"} |= "error" | json
//          ^^^^^^^^^^^^^^^^ ^^^^^^^^^^^^^^^^^^
//          stream selector  pipeline
func SplitQueryParts(query string) (streamSelector string, pipeline string, err error) {
	query = strings.TrimSpace(query)

	// Find the closing } of the stream selector
	if !strings.HasPrefix(query, "{") {
		return "", "", fmt.Errorf("query must start with stream selector {...}")
	}

	depth := 0
	selectorEnd := -1

	for i, ch := range query {
		if ch == '{' {
			depth++
		} else if ch == '}' {
			depth--
			if depth == 0 {
				selectorEnd = i + 1
				break
			}
		}
	}

	if selectorEnd == -1 {
		return "", "", fmt.Errorf("unclosed stream selector")
	}

	streamSelector = query[:selectorEnd]
	pipeline = strings.TrimSpace(query[selectorEnd:])

	return streamSelector, pipeline, nil
}

// tryParseLabelManipulation tries to parse label manipulation: drop label1, label2 | keep label3
func tryParseLabelManipulation(input string) (*LabelManipulation, int) {
	input = strings.TrimSpace(input)

	var operation string
	var rest string

	// Check for drop or keep
	if strings.HasPrefix(input, "drop ") {
		operation = "drop"
		rest = strings.TrimSpace(input[5:])
	} else if strings.HasPrefix(input, "keep ") {
		operation = "keep"
		rest = strings.TrimSpace(input[5:])
	} else {
		return nil, 0
	}

	// Parse comma-separated list of labels
	labels := make([]string, 0)
	consumed := len(input) - len(rest)

	// Parse labels until we hit | or end of string
	for {
		rest = strings.TrimSpace(rest)
		if rest == "" || strings.HasPrefix(rest, "|") {
			break
		}

		// Find next label (ends with comma, |, or end of string)
		labelEnd := strings.IndexAny(rest, ",|")
		if labelEnd == -1 {
			// Last label
			labels = append(labels, strings.TrimSpace(rest))
			consumed = len(input)
			break
		}

		label := strings.TrimSpace(rest[:labelEnd])
		if label != "" {
			labels = append(labels, label)
		}

		// Skip the comma
		if rest[labelEnd] == ',' {
			rest = rest[labelEnd+1:]
			consumed = len(input) - len(rest)
		} else {
			// Hit a |, stop here
			consumed = len(input) - len(rest)
			break
		}
	}

	if len(labels) == 0 {
		return nil, 0
	}

	return &LabelManipulation{
		Operation: operation,
		Labels:    labels,
	}, consumed
}

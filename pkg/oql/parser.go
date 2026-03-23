package oql

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Parser parses OQL query strings
type Parser struct {
	input string
	pos   int
}

// NewParser creates a new OQL parser
func NewParser(input string) *Parser {
	return &Parser{
		input: strings.TrimSpace(input),
		pos:   0,
	}
}

// Parse parses the query string into a Query AST
func (p *Parser) Parse() (*Query, error) {
	query := &Query{
		Operations: make([]Operation, 0),
	}

	// Parse signal declaration
	if !strings.HasPrefix(p.input, "signal=") {
		return nil, fmt.Errorf("query must start with 'signal='")
	}

	p.pos = 7 // skip "signal="
	signalStr := p.readUntil([]string{"|", " where ", " expand ", " correlate ", " get_exemplars", " switch_context ", " extract ", " filter ", " limit ", " aggregate ", " avg ", " min ", " max ", " count ", " sum ", " group ", " since ", " between ", "\n", ""})
	signalStr = strings.TrimSpace(signalStr)

	switch signalStr {
	case "metrics":
		query.Signal = SignalMetrics
	case "logs":
		query.Signal = SignalLogs
	case "spans":
		query.Signal = SignalSpans
	case "traces":
		query.Signal = SignalTraces
	default:
		return nil, fmt.Errorf("invalid signal type: %s", signalStr)
	}

	// Parse operations
	for p.pos < len(p.input) {
		p.skipWhitespace()
		if p.pos >= len(p.input) {
			break
		}

		// Skip pipe separator
		if p.peek() == '|' {
			p.pos++
			p.skipWhitespace()
		}

		op, err := p.parseOperation()
		if err != nil {
			return nil, err
		}
		if op != nil {
			query.Operations = append(query.Operations, op)
		}
	}

	return query, nil
}

// parseOperation parses a single operation
func (p *Parser) parseOperation() (Operation, error) {
	p.skipWhitespace()
	if p.pos >= len(p.input) {
		return nil, nil
	}

	// Peek at the operation keyword
	word := p.peekWord()

	switch word {
	case "where":
		return p.parseWhere()
	case "expand":
		return p.parseExpand()
	case "correlate":
		return p.parseCorrelate()
	case "get_exemplars":
		return p.parseGetExemplars()
	case "switch_context":
		return p.parseSwitchContext()
	case "extract":
		return p.parseExtract()
	case "filter":
		return p.parseFilter()
	case "limit":
		return p.parseLimit()
	case "aggregate", "avg", "min", "max", "count", "sum":
		return p.parseAggregate()
	case "group":
		return p.parseGroupBy()
	case "since":
		return p.parseSince()
	case "between":
		return p.parseBetween()
	default:
		return nil, fmt.Errorf("unknown operation: %s", word)
	}
}

// parseWhere parses a where clause
func (p *Parser) parseWhere() (Operation, error) {
	p.consumeWord("where")
	p.skipWhitespace()

	condition, err := p.parseCondition()
	if err != nil {
		return nil, err
	}

	return &WhereOp{Condition: condition}, nil
}

// parseCondition parses a condition expression
func (p *Parser) parseCondition() (Condition, error) {
	// Read until pipe, operation keyword, or end
	condStr := p.readUntil([]string{"|", " expand ", " correlate ", " get_exemplars", " switch_context ", " extract ", " filter ", " limit ", " aggregate ", " avg ", " min ", " max ", " count ", " sum ", " group ", " since ", " between ", "\n", ""})
	condStr = strings.TrimSpace(condStr)

	// Simple parsing: split by "and" and "or"
	if strings.Contains(condStr, " and ") {
		parts := strings.Split(condStr, " and ")
		conditions := make([]Condition, 0)
		for _, part := range parts {
			cond, err := p.parseSingleCondition(strings.TrimSpace(part))
			if err != nil {
				return nil, err
			}
			conditions = append(conditions, cond)
		}
		return &AndCondition{Conditions: conditions}, nil
	}

	if strings.Contains(condStr, " or ") {
		parts := strings.Split(condStr, " or ")
		conditions := make([]Condition, 0)
		for _, part := range parts {
			cond, err := p.parseSingleCondition(strings.TrimSpace(part))
			if err != nil {
				return nil, err
			}
			conditions = append(conditions, cond)
		}
		return &OrCondition{Conditions: conditions}, nil
	}

	return p.parseSingleCondition(condStr)
}

// parseSingleCondition parses a single binary condition
func (p *Parser) parseSingleCondition(s string) (Condition, error) {
	// Try different operators
	operators := []string{"==", "!=", ">=", "<=", ">", "<"}
	for _, op := range operators {
		if idx := strings.Index(s, op); idx != -1 {
			left := strings.TrimSpace(s[:idx])
			right := strings.TrimSpace(s[idx+len(op):])

			// Check for empty parts
			if left == "" {
				return nil, fmt.Errorf("invalid condition: missing left operand in %s", s)
			}
			if right == "" {
				return nil, fmt.Errorf("invalid condition: missing right operand in %s", s)
			}

			// Parse the right side value
			value := p.parseValue(right)

			return &BinaryCondition{
				Left:     left,
				Operator: op,
				Right:    value,
			}, nil
		}
	}

	return nil, fmt.Errorf("invalid condition: %s", s)
}

// parseValue parses a value (string, number, duration, boolean)
func (p *Parser) parseValue(s string) interface{} {
	s = strings.TrimSpace(s)

	// Remove quotes for strings
	if strings.HasPrefix(s, `"`) && strings.HasSuffix(s, `"`) {
		return strings.Trim(s, `"`)
	}

	// Try boolean
	if s == "true" {
		return true
	}
	if s == "false" {
		return false
	}

	// Try duration
	if strings.HasSuffix(s, "ms") || strings.HasSuffix(s, "s") || strings.HasSuffix(s, "m") || strings.HasSuffix(s, "h") {
		if d, err := parseDuration(s); err == nil {
			return d
		}
	}

	// Try integer
	if i, err := strconv.ParseInt(s, 10, 64); err == nil {
		return i
	}

	// Try float
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return f
	}

	// Default to string
	return s
}

// parseExpand parses an expand operation
func (p *Parser) parseExpand() (Operation, error) {
	p.consumeWord("expand")
	p.skipWhitespace()

	expandType := p.readWord()
	if expandType != "trace" {
		return nil, fmt.Errorf("expand only supports 'trace', got: %s", expandType)
	}

	return &ExpandOp{Type: "trace"}, nil
}

// parseCorrelate parses a correlate operation
func (p *Parser) parseCorrelate() (Operation, error) {
	p.consumeWord("correlate")
	p.skipWhitespace()

	signalsStr := p.readUntil([]string{"|", " expand ", " where ", " get_exemplars", " switch_context ", " extract ", " filter ", " limit ", " aggregate ", " avg ", " min ", " max ", " count ", " sum ", " group ", " since ", " between ", "\n", ""})
	signalsStr = strings.TrimSpace(signalsStr)

	// Split by comma
	signalStrs := strings.Split(signalsStr, ",")
	signals := make([]SignalType, 0)
	for _, s := range signalStrs {
		s = strings.TrimSpace(s)
		switch s {
		case "metrics":
			signals = append(signals, SignalMetrics)
		case "logs":
			signals = append(signals, SignalLogs)
		case "spans":
			signals = append(signals, SignalSpans)
		case "traces":
			signals = append(signals, SignalTraces)
		default:
			return nil, fmt.Errorf("invalid signal type in correlate: %s", s)
		}
	}

	return &CorrelateOp{Signals: signals}, nil
}

// parseGetExemplars parses a get_exemplars operation
func (p *Parser) parseGetExemplars() (Operation, error) {
	p.consumeWord("get_exemplars")
	p.skipWhitespace()

	// Consume optional parentheses
	if p.peek() == '(' {
		p.pos++
		if p.peek() == ')' {
			p.pos++
		}
	}

	return &GetExemplarsOp{}, nil
}

// parseSwitchContext parses a switch_context operation
func (p *Parser) parseSwitchContext() (Operation, error) {
	p.consumeWord("switch_context")
	p.skipWhitespace()

	// Expect signal=<type>
	if !strings.HasPrefix(p.input[p.pos:], "signal=") {
		return nil, fmt.Errorf("switch_context requires 'signal=' parameter")
	}
	p.pos += 7 // skip "signal="

	signalStr := p.readUntil([]string{"|", " where ", " expand ", " correlate ", " get_exemplars", " extract ", " filter ", " limit ", " aggregate ", " avg ", " min ", " max ", " count ", " sum ", " group ", " since ", " between ", "\n", ""})
	signalStr = strings.TrimSpace(signalStr)

	var signal SignalType
	switch signalStr {
	case "metrics":
		signal = SignalMetrics
	case "logs":
		signal = SignalLogs
	case "spans":
		signal = SignalSpans
	case "traces":
		signal = SignalTraces
	default:
		return nil, fmt.Errorf("invalid signal type: %s", signalStr)
	}

	return &SwitchContextOp{Signal: signal}, nil
}

// parseExtract parses an extract operation
func (p *Parser) parseExtract() (Operation, error) {
	p.consumeWord("extract")
	p.skipWhitespace()

	// Read field
	field := p.readWord()
	if field == "" {
		return nil, fmt.Errorf("extract requires a field name")
	}

	p.skipWhitespace()

	// Expect "as"
	if p.peekWord() != "as" {
		return nil, fmt.Errorf("extract requires 'as' keyword")
	}
	p.consumeWord("as")
	p.skipWhitespace()

	// Read alias
	alias := p.readWord()
	if alias == "" {
		return nil, fmt.Errorf("extract requires an alias after 'as'")
	}

	return &ExtractOp{Field: field, Alias: alias}, nil
}

// parseFilter parses a filter operation
func (p *Parser) parseFilter() (Operation, error) {
	p.consumeWord("filter")
	p.skipWhitespace()

	condition, err := p.parseCondition()
	if err != nil {
		return nil, err
	}

	return &FilterOp{Condition: condition}, nil
}

// parseLimit parses a limit operation
func (p *Parser) parseLimit() (Operation, error) {
	p.consumeWord("limit")
	p.skipWhitespace()

	countStr := p.readWord()
	count, err := strconv.Atoi(countStr)
	if err != nil {
		return nil, fmt.Errorf("invalid limit count: %s", countStr)
	}

	return &LimitOp{Count: count}, nil
}

// Helper methods

func (p *Parser) peek() byte {
	if p.pos >= len(p.input) {
		return 0
	}
	return p.input[p.pos]
}

func (p *Parser) peekWord() string {
	start := p.pos
	for p.pos < len(p.input) && !isWhitespace(p.input[p.pos]) && p.input[p.pos] != '|' && p.input[p.pos] != '(' {
		p.pos++
	}
	word := p.input[start:p.pos]
	p.pos = start // reset position
	return word
}

func (p *Parser) readWord() string {
	start := p.pos
	for p.pos < len(p.input) && !isWhitespace(p.input[p.pos]) && p.input[p.pos] != '|' && p.input[p.pos] != '(' {
		p.pos++
	}
	return p.input[start:p.pos]
}

func (p *Parser) consumeWord(word string) {
	p.pos += len(word)
}

func (p *Parser) readUntil(delimiters []string) string {
	start := p.pos
	for p.pos < len(p.input) {
		for _, delim := range delimiters {
			if delim == "" {
				if p.pos >= len(p.input) {
					result := p.input[start:p.pos]
					return result
				}
			} else if strings.HasPrefix(p.input[p.pos:], delim) {
				result := p.input[start:p.pos]
				return result
			}
		}
		p.pos++
	}
	return p.input[start:p.pos]
}

func (p *Parser) skipWhitespace() {
	for p.pos < len(p.input) && isWhitespace(p.input[p.pos]) {
		p.pos++
	}
}

func isWhitespace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}

// parseDuration parses duration strings like "500ms", "2s", "5m"
func parseDuration(s string) (time.Duration, error) {
	// Handle simple cases
	if strings.HasSuffix(s, "ms") {
		val := strings.TrimSuffix(s, "ms")
		ms, err := strconv.ParseInt(val, 10, 64)
		if err != nil {
			return 0, err
		}
		return time.Duration(ms) * time.Millisecond, nil
	}

	// Use Go's duration parser for other formats
	return time.ParseDuration(s)
}

// parseAggregate parses an aggregate operation
func (p *Parser) parseAggregate() (Operation, error) {
	// Can be: aggregate avg(duration), avg(duration), count(), etc.
	funcName := p.readWord()

	// If it's "aggregate", read the actual function
	if funcName == "aggregate" {
		p.skipWhitespace()
		funcName = p.readWord()
	}

	p.skipWhitespace()

	var field string
	var alias string

	// Check if there's a parenthesis (for field specification)
	if p.peek() == '(' {
		p.pos++ // consume '('
		field = p.readUntil([]string{")", " as "})
		field = strings.TrimSpace(field)

		if p.peek() == ')' {
			p.pos++ // consume ')'
		}
	}

	p.skipWhitespace()

	// Check for alias
	if p.peekWord() == "as" {
		p.consumeWord("as")
		p.skipWhitespace()
		alias = p.readWord()
	}

	return &AggregateOp{
		Function: funcName,
		Field:    field,
		Alias:    alias,
	}, nil
}

// parseGroupBy parses a group by operation
func (p *Parser) parseGroupBy() (Operation, error) {
	p.consumeWord("group")
	p.skipWhitespace()

	// Expect "by"
	if p.peekWord() != "by" {
		return nil, fmt.Errorf("group requires 'by' keyword")
	}
	p.consumeWord("by")
	p.skipWhitespace()

	// Read fields (comma-separated)
	fieldsStr := p.readUntil([]string{"|", " aggregate ", " avg ", " min ", " max ", " count ", " sum ", " limit ", "\n", ""})
	fieldsStr = strings.TrimSpace(fieldsStr)

	// Split by comma
	fieldStrs := strings.Split(fieldsStr, ",")
	fields := make([]string, 0)
	for _, f := range fieldStrs {
		fields = append(fields, strings.TrimSpace(f))
	}

	return &GroupByOp{Fields: fields}, nil
}

// parseSince parses a since time range operation
func (p *Parser) parseSince() (Operation, error) {
	p.consumeWord("since")
	p.skipWhitespace()

	// Read duration or timestamp
	duration := p.readWord()

	return &SinceOp{Duration: duration}, nil
}

// parseBetween parses a between time range operation
func (p *Parser) parseBetween() (Operation, error) {
	p.consumeWord("between")
	p.skipWhitespace()

	// Read start time
	start := p.readWord()
	p.skipWhitespace()

	// Expect "and"
	if p.peekWord() != "and" {
		return nil, fmt.Errorf("between requires 'and' keyword")
	}
	p.consumeWord("and")
	p.skipWhitespace()

	// Read end time
	end := p.readWord()

	return &BetweenOp{Start: start, End: end}, nil
}

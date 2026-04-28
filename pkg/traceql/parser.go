package traceql

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/promql/parser"
)

// Parser parses TraceQL queries
type Parser struct {
	lexer   *Lexer
	current Token
	peek    Token
}

// NewParser creates a new TraceQL parser
func NewParser(input string) *Parser {
	lexer := NewLexer(input)
	p := &Parser{
		lexer: lexer,
	}
	// Load first two tokens
	p.nextToken()
	p.nextToken()
	return p
}

// Parse parses the TraceQL query
func (p *Parser) Parse() (*Query, error) {
	// First try to parse with Prometheus parser to detect scalar arithmetic
	// This handles connection tests like 1+1
	promParser := parser.NewParser(parser.Options{})
	expr, err := promParser.ParseExpr(p.lexer.input)
	if err == nil {
		// Check if it's a scalar arithmetic expression
		if scalarExpr, ok := p.tryParseScalarExpr(expr); ok {
			return &Query{Expr: scalarExpr}, nil
		}
	}

	// Check if this is an aggregation query
	// Examples: count() by (span.service.name), sum(duration) by (span.http.method)
	if p.current.Type == TokenCount || p.current.Type == TokenSum ||
		p.current.Type == TokenAvg || p.current.Type == TokenMin ||
		p.current.Type == TokenMax {
		return p.parseAggregateExpr()
	}

	// Otherwise it's a span filter query: {conditions}
	return p.parseSpanFilterQuery()
}

// parseSpanFilterQuery parses a span filter query
// Example: {span.http.status_code = 500 && duration > 100ms}
func (p *Parser) parseSpanFilterQuery() (*Query, error) {
	// Expect opening brace
	if !p.expectCurrent(TokenLBrace) {
		return nil, fmt.Errorf("expected '{' at position %d, got %v", p.current.Pos, p.current.Type)
	}
	p.nextToken()

	// Parse conditions
	conditions, err := p.parseConditions()
	if err != nil {
		return nil, err
	}

	// Expect closing brace
	if !p.expectCurrent(TokenRBrace) {
		return nil, fmt.Errorf("expected '}' at position %d, got %v", p.current.Pos, p.current.Type)
	}
	p.nextToken()

	return &Query{
		Expr: &SpanFilterExpr{
			Conditions: conditions,
		},
	}, nil
}

// parseConditions parses multiple conditions separated by && or ||
func (p *Parser) parseConditions() ([]Condition, error) {
	conditions := []Condition{}

	for {
		// Parse a single condition
		condition, err := p.parseCondition()
		if err != nil {
			return nil, err
		}
		conditions = append(conditions, condition)

		// Check for logical operator (&&, ||)
		if p.current.Type == TokenAnd || p.current.Type == TokenOr {
			p.nextToken() // consume operator
			continue      // parse next condition
		}

		// No more conditions
		break
	}

	return conditions, nil
}

// parseCondition parses a single condition
// Examples: duration > 100ms, span.http.status_code = 500, name = "HTTP GET"
func (p *Parser) parseCondition() (Condition, error) {
	// Parse field expression
	field, err := p.parseFieldExpr()
	if err != nil {
		return Condition{}, err
	}

	// Parse operator
	operator, err := p.parseOperator()
	if err != nil {
		return Condition{}, err
	}

	// Parse value
	value, err := p.parseValue()
	if err != nil {
		return Condition{}, err
	}

	return Condition{
		Field:    field,
		Operator: operator,
		Value:    value,
	}, nil
}

// parseFieldExpr parses a field expression
// Examples: duration, name, span.http.method, resource.service.name
func (p *Parser) parseFieldExpr() (FieldExpr, error) {
	// Check for intrinsic fields (no prefix)
	if p.current.Type == TokenIdent {
		name := strings.ToLower(p.current.Value)
		if IsIntrinsic(name) {
			p.nextToken()
			return FieldExpr{
				Type: "intrinsic",
				Name: name,
			}, nil
		}
	}

	// Check for span. or resource. prefix
	var fieldType string
	if p.current.Type == TokenSpan {
		fieldType = "span"
		p.nextToken()
	} else if p.current.Type == TokenResource {
		fieldType = "resource"
		p.nextToken()
	} else {
		return FieldExpr{}, fmt.Errorf("expected field expression at position %d, got %v", p.current.Pos, p.current.Type)
	}

	// Expect dot
	if !p.expectCurrent(TokenDot) {
		return FieldExpr{}, fmt.Errorf("expected '.' after %s at position %d", fieldType, p.current.Pos)
	}
	p.nextToken()

	// Parse attribute name (can be dotted like http.status_code)
	attrName, err := p.parseDottedIdentifier()
	if err != nil {
		return FieldExpr{}, err
	}

	return FieldExpr{
		Type: fieldType,
		Name: attrName,
	}, nil
}

// parseDottedIdentifier parses a dotted identifier
// Example: http.status_code, service.name
func (p *Parser) parseDottedIdentifier() (string, error) {
	parts := []string{}

	// First part
	if p.current.Type != TokenIdent {
		return "", fmt.Errorf("expected identifier at position %d, got %v", p.current.Pos, p.current.Type)
	}
	parts = append(parts, p.current.Value)
	p.nextToken()

	// Additional parts (separated by dots)
	for p.current.Type == TokenDot {
		p.nextToken() // consume dot
		if p.current.Type != TokenIdent {
			return "", fmt.Errorf("expected identifier after '.' at position %d", p.current.Pos)
		}
		parts = append(parts, p.current.Value)
		p.nextToken()
	}

	return strings.Join(parts, "."), nil
}

// parseOperator parses a comparison operator
func (p *Parser) parseOperator() (string, error) {
	var op string
	switch p.current.Type {
	case TokenEq:
		op = "="
	case TokenNotEq:
		op = "!="
	case TokenRegexp:
		op = "=~"
	case TokenNotRegexp:
		op = "!~"
	case TokenGT:
		op = ">"
	case TokenLT:
		op = "<"
	case TokenGTE:
		op = ">="
	case TokenLTE:
		op = "<="
	default:
		return "", fmt.Errorf("expected operator at position %d, got %v", p.current.Pos, p.current.Type)
	}

	p.nextToken()
	return op, nil
}

// parseValue parses a value (string, number, or duration)
func (p *Parser) parseValue() (interface{}, error) {
	switch p.current.Type {
	case TokenString:
		value := p.current.Value
		p.nextToken()
		return value, nil

	case TokenNumber:
		valueStr := p.current.Value
		p.nextToken()
		// Try to parse as float
		value, err := strconv.ParseFloat(valueStr, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid number: %s", valueStr)
		}
		return value, nil

	case TokenDuration:
		durationStr := p.current.Value
		p.nextToken()
		// Parse using Prometheus duration parser
		duration, err := model.ParseDuration(durationStr)
		if err != nil {
			return nil, fmt.Errorf("invalid duration: %s", durationStr)
		}
		return time.Duration(duration), nil

	case TokenIdent:
		// Could be a status value (unset, ok, error) or boolean (true, false)
		value := strings.ToLower(p.current.Value)
		p.nextToken()

		switch value {
		case "unset", "ok", "error":
			return StatusValue(value), nil
		case "true":
			return true, nil
		case "false":
			return false, nil
		default:
			return value, nil // Return as string
		}

	default:
		return nil, fmt.Errorf("expected value at position %d, got %v", p.current.Pos, p.current.Type)
	}
}

// parseAggregateExpr parses an aggregation expression
// Example: count() by (span.service.name)
func (p *Parser) parseAggregateExpr() (*Query, error) {
	// Get function name
	function := p.current.Value
	p.nextToken()

	// Expect opening paren
	if !p.expectCurrent(TokenLParen) {
		return nil, fmt.Errorf("expected '(' after %s", function)
	}
	p.nextToken()

	// Expect closing paren (for now, we only support count(), not count(field))
	if !p.expectCurrent(TokenRParen) {
		return nil, fmt.Errorf("expected ')' after %s(", function)
	}
	p.nextToken()

	// Check for "by" clause
	var grouping []string
	if p.current.Type == TokenBy {
		p.nextToken()

		// Expect opening paren
		if !p.expectCurrent(TokenLParen) {
			return nil, fmt.Errorf("expected '(' after 'by'")
		}
		p.nextToken()

		// Parse grouping fields
		for {
			// Parse dotted identifier (span.service.name, resource.environment, etc.)
			field, err := p.parseGroupingField()
			if err != nil {
				return nil, err
			}
			grouping = append(grouping, field)

			// Check for comma
			if p.current.Type == TokenComma {
				p.nextToken()
				continue
			}

			break
		}

		// Expect closing paren
		if !p.expectCurrent(TokenRParen) {
			return nil, fmt.Errorf("expected ')' after grouping fields")
		}
		p.nextToken()
	}

	return &Query{
		Expr: &AggregateExpr{
			Function: function,
			Inner:    nil, // For simple count(), no inner expression
			Grouping: grouping,
		},
	}, nil
}

// parseGroupingField parses a grouping field
// Example: span.service.name, resource.environment, duration
func (p *Parser) parseGroupingField() (string, error) {
	// Check for intrinsic fields
	if p.current.Type == TokenIdent {
		name := strings.ToLower(p.current.Value)
		if IsIntrinsic(name) {
			p.nextToken()
			return name, nil
		}
	}

	// Check for span. or resource. prefix
	var prefix string
	if p.current.Type == TokenSpan {
		prefix = "span."
		p.nextToken()
	} else if p.current.Type == TokenResource {
		prefix = "resource."
		p.nextToken()
	} else {
		return "", fmt.Errorf("expected grouping field at position %d", p.current.Pos)
	}

	// Expect dot
	if !p.expectCurrent(TokenDot) {
		return "", fmt.Errorf("expected '.' after prefix")
	}
	p.nextToken()

	// Parse dotted identifier
	attrName, err := p.parseDottedIdentifier()
	if err != nil {
		return "", err
	}

	return prefix + attrName, nil
}

// nextToken advances to the next token
func (p *Parser) nextToken() {
	p.current = p.peek
	p.peek = p.lexer.NextToken()
}

// expectCurrent checks if the current token type matches
func (p *Parser) expectCurrent(t TokenType) bool {
	return p.current.Type == t
}

// tryParseScalarExpr attempts to parse a scalar arithmetic expression
// This handles connection tests like 1+1 from Grafana
func (p *Parser) tryParseScalarExpr(expr parser.Expr) (*ScalarExpr, bool) {
	// Handle BinaryExpr (e.g., 1+1)
	if be, ok := expr.(*parser.BinaryExpr); ok {
		lhsVal, lhsOk := p.extractScalarValue(be.LHS)
		rhsVal, rhsOk := p.extractScalarValue(be.RHS)

		if lhsOk && rhsOk {
			var result float64
			switch be.Op {
			case parser.ADD:
				result = lhsVal + rhsVal
			case parser.SUB:
				result = lhsVal - rhsVal
			case parser.MUL:
				result = lhsVal * rhsVal
			case parser.DIV:
				if rhsVal == 0 {
					return nil, false
				}
				result = lhsVal / rhsVal
			default:
				return nil, false
			}
			return &ScalarExpr{Value: result}, true
		}
	}

	// Handle single scalar value
	if val, ok := p.extractScalarValue(expr); ok {
		return &ScalarExpr{Value: val}, true
	}

	return nil, false
}

// extractScalarValue extracts a scalar value from a Prometheus expression
func (p *Parser) extractScalarValue(expr parser.Expr) (float64, bool) {
	// NumberLiteral: 1, 2.5, etc.
	if num, ok := expr.(*parser.NumberLiteral); ok {
		return num.Val, true
	}

	// ParenExpr: unwrap parentheses
	if paren, ok := expr.(*parser.ParenExpr); ok {
		return p.extractScalarValue(paren.Expr)
	}

	return 0, false
}

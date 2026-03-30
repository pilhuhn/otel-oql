package traceql

import (
	"testing"
)

func TestLexer_SimpleTokens(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect []TokenType
	}{
		{
			name:  "braces",
			input: "{}",
			expect: []TokenType{
				TokenLBrace, TokenRBrace, TokenEOF,
			},
		},
		{
			name:  "parentheses",
			input: "()",
			expect: []TokenType{
				TokenLParen, TokenRParen, TokenEOF,
			},
		},
		{
			name:  "operators",
			input: "= != > < >= <=",
			expect: []TokenType{
				TokenEq, TokenNotEq, TokenGT, TokenLT, TokenGTE, TokenLTE, TokenEOF,
			},
		},
		{
			name:  "regex operators",
			input: "=~ !~",
			expect: []TokenType{
				TokenRegexp, TokenNotRegexp, TokenEOF,
			},
		},
		{
			name:  "logical operators",
			input: "&& ||",
			expect: []TokenType{
				TokenAnd, TokenOr, TokenEOF,
			},
		},
		{
			name:  "dot notation",
			input: "span.http.method",
			expect: []TokenType{
				TokenSpan, TokenDot, TokenIdent, TokenDot, TokenIdent, TokenEOF,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lexer := NewLexer(tt.input)
			tokens := lexer.AllTokens()

			if len(tokens) != len(tt.expect) {
				t.Fatalf("expected %d tokens, got %d", len(tt.expect), len(tokens))
			}

			for i, expectedType := range tt.expect {
				if tokens[i].Type != expectedType {
					t.Errorf("token %d: expected type %v, got %v (value: %q)",
						i, expectedType, tokens[i].Type, tokens[i].Value)
				}
			}
		})
	}
}

func TestLexer_Strings(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{
			name:   "double quoted string",
			input:  `"hello world"`,
			expect: "hello world",
		},
		{
			name:   "single quoted string",
			input:  `'hello world'`,
			expect: "hello world",
		},
		{
			name:   "string with escaped quote",
			input:  `"hello \"world\""`,
			expect: `hello \"world\"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lexer := NewLexer(tt.input)
			token := lexer.NextToken()

			if token.Type != TokenString {
				t.Fatalf("expected TokenString, got %v", token.Type)
			}

			if token.Value != tt.expect {
				t.Errorf("expected value %q, got %q", tt.expect, token.Value)
			}
		})
	}
}

func TestLexer_Numbers(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		expectType TokenType
		expectVal  string
	}{
		{
			name:       "integer",
			input:      "123",
			expectType: TokenNumber,
			expectVal:  "123",
		},
		{
			name:       "float",
			input:      "123.456",
			expectType: TokenNumber,
			expectVal:  "123.456",
		},
		{
			name:       "duration milliseconds",
			input:      "100ms",
			expectType: TokenDuration,
			expectVal:  "100ms",
		},
		{
			name:       "duration seconds",
			input:      "5s",
			expectType: TokenDuration,
			expectVal:  "5s",
		},
		{
			name:       "duration minutes",
			input:      "10m",
			expectType: TokenDuration,
			expectVal:  "10m",
		},
		{
			name:       "duration hours",
			input:      "1h",
			expectType: TokenDuration,
			expectVal:  "1h",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lexer := NewLexer(tt.input)
			token := lexer.NextToken()

			if token.Type != tt.expectType {
				t.Fatalf("expected %v, got %v", tt.expectType, token.Type)
			}

			if token.Value != tt.expectVal {
				t.Errorf("expected value %q, got %q", tt.expectVal, token.Value)
			}
		})
	}
}

func TestLexer_Keywords(t *testing.T) {
	tests := []struct {
		input  string
		expect TokenType
	}{
		{"span", TokenSpan},
		{"resource", TokenResource},
		{"by", TokenBy},
		{"without", TokenWithout},
		{"count", TokenCount},
		{"sum", TokenSum},
		{"avg", TokenAvg},
		{"min", TokenMin},
		{"max", TokenMax},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			lexer := NewLexer(tt.input)
			token := lexer.NextToken()

			if token.Type != tt.expect {
				t.Errorf("expected %v, got %v", tt.expect, token.Type)
			}
		})
	}
}

func TestLexer_TraceQLQuery(t *testing.T) {
	input := `{span.http.status_code = 500 && duration > 100ms}`

	expected := []struct {
		tokenType TokenType
		value     string
	}{
		{TokenLBrace, "{"},
		{TokenSpan, "span"},
		{TokenDot, "."},
		{TokenIdent, "http"},
		{TokenDot, "."},
		{TokenIdent, "status_code"},
		{TokenEq, "="},
		{TokenNumber, "500"},
		{TokenAnd, "&&"},
		{TokenIdent, "duration"},
		{TokenGT, ">"},
		{TokenDuration, "100ms"},
		{TokenRBrace, "}"},
		{TokenEOF, ""},
	}

	lexer := NewLexer(input)
	tokens := lexer.AllTokens()

	if len(tokens) != len(expected) {
		t.Fatalf("expected %d tokens, got %d", len(expected), len(tokens))
	}

	for i, exp := range expected {
		if tokens[i].Type != exp.tokenType {
			t.Errorf("token %d: expected type %v, got %v",
				i, exp.tokenType, tokens[i].Type)
		}
	}
}

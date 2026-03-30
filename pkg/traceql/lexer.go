package traceql

import (
	"fmt"
	"strings"
	"unicode"
)

// Token types
type TokenType int

const (
	TokenEOF TokenType = iota
	TokenError

	// Literals
	TokenIdent   // identifier (field names)
	TokenString  // string literal
	TokenNumber  // number literal
	TokenDuration // duration literal (100ms, 5s, etc.)

	// Delimiters
	TokenLBrace // {
	TokenRBrace // }
	TokenLParen // (
	TokenRParen // )
	TokenComma  // ,
	TokenDot    // .

	// Operators
	TokenEq        // =
	TokenNotEq     // !=
	TokenRegexp    // =~
	TokenNotRegexp // !~
	TokenGT        // >
	TokenLT        // <
	TokenGTE       // >=
	TokenLTE       // <=

	// Logical operators
	TokenAnd // &&
	TokenOr  // ||

	// Keywords
	TokenSpan     // span
	TokenResource // resource
	TokenBy       // by
	TokenWithout  // without

	// Functions
	TokenCount // count
	TokenSum   // sum
	TokenAvg   // avg
	TokenMin   // min
	TokenMax   // max
)

// Token represents a lexical token
type Token struct {
	Type  TokenType
	Value string
	Pos   int
}

// Lexer tokenizes TraceQL queries
type Lexer struct {
	input string
	pos   int
	start int
	width int
}

// NewLexer creates a new lexer for TraceQL
func NewLexer(input string) *Lexer {
	return &Lexer{
		input: strings.TrimSpace(input),
		pos:   0,
		start: 0,
		width: 0,
	}
}

// NextToken returns the next token from the input
func (l *Lexer) NextToken() Token {
	l.skipWhitespace()

	if l.pos >= len(l.input) {
		return Token{Type: TokenEOF, Pos: l.pos}
	}

	l.start = l.pos
	ch := l.peek()

	switch {
	// Single character tokens
	case ch == '{':
		l.advance()
		return Token{Type: TokenLBrace, Value: "{", Pos: l.start}
	case ch == '}':
		l.advance()
		return Token{Type: TokenRBrace, Value: "}", Pos: l.start}
	case ch == '(':
		l.advance()
		return Token{Type: TokenLParen, Value: "(", Pos: l.start}
	case ch == ')':
		l.advance()
		return Token{Type: TokenRParen, Value: ")", Pos: l.start}
	case ch == ',':
		l.advance()
		return Token{Type: TokenComma, Value: ",", Pos: l.start}
	case ch == '.':
		l.advance()
		return Token{Type: TokenDot, Value: ".", Pos: l.start}

	// Operators
	case ch == '=':
		l.advance()
		if l.peek() == '~' {
			l.advance()
			return Token{Type: TokenRegexp, Value: "=~", Pos: l.start}
		}
		return Token{Type: TokenEq, Value: "=", Pos: l.start}

	case ch == '!':
		l.advance()
		if l.peek() == '=' {
			l.advance()
			return Token{Type: TokenNotEq, Value: "!=", Pos: l.start}
		}
		if l.peek() == '~' {
			l.advance()
			return Token{Type: TokenNotRegexp, Value: "!~", Pos: l.start}
		}
		return Token{Type: TokenError, Value: "unexpected '!'", Pos: l.start}

	case ch == '>':
		l.advance()
		if l.peek() == '=' {
			l.advance()
			return Token{Type: TokenGTE, Value: ">=", Pos: l.start}
		}
		return Token{Type: TokenGT, Value: ">", Pos: l.start}

	case ch == '<':
		l.advance()
		if l.peek() == '=' {
			l.advance()
			return Token{Type: TokenLTE, Value: "<=", Pos: l.start}
		}
		return Token{Type: TokenLT, Value: "<", Pos: l.start}

	case ch == '&':
		l.advance()
		if l.peek() == '&' {
			l.advance()
			return Token{Type: TokenAnd, Value: "&&", Pos: l.start}
		}
		return Token{Type: TokenError, Value: "unexpected '&' (did you mean '&&'?)", Pos: l.start}

	case ch == '|':
		l.advance()
		if l.peek() == '|' {
			l.advance()
			return Token{Type: TokenOr, Value: "||", Pos: l.start}
		}
		return Token{Type: TokenError, Value: "unexpected '|' (did you mean '||'?)", Pos: l.start}

	// String literals
	case ch == '"' || ch == '\'':
		return l.scanString()

	// Numbers and durations
	case unicode.IsDigit(rune(ch)):
		return l.scanNumber()

	// Identifiers and keywords
	case unicode.IsLetter(rune(ch)) || ch == '_':
		return l.scanIdentifier()

	default:
		l.advance()
		return Token{Type: TokenError, Value: fmt.Sprintf("unexpected character: %c", ch), Pos: l.start}
	}
}

// peek returns the current character without advancing
func (l *Lexer) peek() byte {
	if l.pos >= len(l.input) {
		return 0
	}
	return l.input[l.pos]
}

// advance moves to the next character
func (l *Lexer) advance() {
	if l.pos < len(l.input) {
		l.width = 1
		l.pos++
	}
}

// skipWhitespace skips whitespace characters
func (l *Lexer) skipWhitespace() {
	for l.pos < len(l.input) && unicode.IsSpace(rune(l.input[l.pos])) {
		l.advance()
	}
}

// scanString scans a string literal
func (l *Lexer) scanString() Token {
	quote := l.peek()
	l.advance() // skip opening quote

	start := l.pos
	for l.pos < len(l.input) {
		ch := l.peek()
		if ch == quote {
			value := l.input[start:l.pos]
			l.advance() // skip closing quote
			return Token{Type: TokenString, Value: value, Pos: l.start}
		}
		if ch == '\\' {
			l.advance() // skip escape character
			if l.pos < len(l.input) {
				l.advance() // skip escaped character
			}
			continue
		}
		l.advance()
	}

	return Token{Type: TokenError, Value: "unterminated string", Pos: l.start}
}

// scanNumber scans a number literal or duration
func (l *Lexer) scanNumber() Token {
	start := l.pos

	// Scan digits
	for l.pos < len(l.input) && unicode.IsDigit(rune(l.input[l.pos])) {
		l.advance()
	}

	// Check for decimal point
	if l.peek() == '.' {
		l.advance()
		for l.pos < len(l.input) && unicode.IsDigit(rune(l.input[l.pos])) {
			l.advance()
		}
	}

	// Check for duration suffix (ms, s, m, h, d)
	if l.pos < len(l.input) {
		ch := l.peek()
		if ch == 'm' || ch == 's' || ch == 'h' || ch == 'd' {
			// Could be duration suffix
			if ch == 'm' {
				l.advance()
				if l.peek() == 's' {
					l.advance()
				}
			} else {
				l.advance()
			}

			value := l.input[start:l.pos]
			return Token{Type: TokenDuration, Value: value, Pos: start}
		}
	}

	value := l.input[start:l.pos]
	return Token{Type: TokenNumber, Value: value, Pos: start}
}

// scanIdentifier scans an identifier or keyword
func (l *Lexer) scanIdentifier() Token {
	start := l.pos

	// First character already validated by caller
	l.advance()

	// Continue scanning alphanumeric and underscore
	for l.pos < len(l.input) {
		ch := l.peek()
		if unicode.IsLetter(rune(ch)) || unicode.IsDigit(rune(ch)) || ch == '_' {
			l.advance()
		} else {
			break
		}
	}

	value := l.input[start:l.pos]

	// Check for keywords
	switch strings.ToLower(value) {
	case "span":
		return Token{Type: TokenSpan, Value: value, Pos: start}
	case "resource":
		return Token{Type: TokenResource, Value: value, Pos: start}
	case "by":
		return Token{Type: TokenBy, Value: value, Pos: start}
	case "without":
		return Token{Type: TokenWithout, Value: value, Pos: start}
	case "count":
		return Token{Type: TokenCount, Value: value, Pos: start}
	case "sum":
		return Token{Type: TokenSum, Value: value, Pos: start}
	case "avg":
		return Token{Type: TokenAvg, Value: value, Pos: start}
	case "min":
		return Token{Type: TokenMin, Value: value, Pos: start}
	case "max":
		return Token{Type: TokenMax, Value: value, Pos: start}
	default:
		return Token{Type: TokenIdent, Value: value, Pos: start}
	}
}

// AllTokens returns all tokens (useful for debugging)
func (l *Lexer) AllTokens() []Token {
	tokens := []Token{}
	for {
		tok := l.NextToken()
		tokens = append(tokens, tok)
		if tok.Type == TokenEOF || tok.Type == TokenError {
			break
		}
	}
	return tokens
}

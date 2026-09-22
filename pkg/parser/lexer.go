package parser

import (
	"fmt"
	"strings"
	"unicode"
)

// TokenType represents a lexical token type in OPS5 syntax.
type TokenType int

const (
	TokenEOF TokenType = iota
	TokenLParen        // (
	TokenRParen        // )
	TokenLBrace        // {
	TokenRBrace        // }
	TokenLBracket      // [
	TokenRBracket      // ]
	TokenArrow         // -->
	TokenNegation      // -
	TokenAttribute     // ^attr
	TokenVariable      // <var>
	TokenSymbol        // symbol
	TokenNumber        // 123, 3.14
	TokenString        // "string"
	TokenOperator      // =, <>, !=, <, <=, >, >=
	TokenLDisj         // <<
	TokenRDisj         // >>
)

func (t TokenType) String() string {
	switch t {
	case TokenEOF:
		return "EOF"
	case TokenLParen:
		return "("
	case TokenRParen:
		return ")"
	case TokenLBrace:
		return "{"
	case TokenRBrace:
		return "}"
	case TokenLBracket:
		return "["
	case TokenRBracket:
		return "]"
	case TokenLDisj:
		return "<<"
	case TokenRDisj:
		return ">>"
	case TokenArrow:
		return "-->"
	case TokenNegation:
		return "-"
	case TokenAttribute:
		return "attribute"
	case TokenVariable:
		return "variable"
	case TokenSymbol:
		return "symbol"
	case TokenNumber:
		return "number"
	case TokenString:
		return "string"
	case TokenOperator:
		return "operator"
	default:
		return "unknown"
	}
}

// Token represents an OPS5 lexical token.
type Token struct {
	Type  TokenType
	Value string
	Line  int
	Col   int
}

// Lexer tokenizes OPS5 source code.
type Lexer struct {
	src    []rune
	cursor int
	line   int
	col    int
}

// NewLexer creates a new Lexer for OPS5 text.
func NewLexer(input string) *Lexer {
	return &Lexer{
		src:    []rune(input),
		cursor: 0,
		line:   1,
		col:    1,
	}
}

func (l *Lexer) peek() rune {
	if l.cursor >= len(l.src) {
		return 0
	}
	return l.src[l.cursor]
}

func (l *Lexer) next() rune {
	if l.cursor >= len(l.src) {
		return 0
	}
	r := l.src[l.cursor]
	l.cursor++
	if r == '\n' {
		l.line++
		l.col = 1
	} else {
		l.col++
	}
	return r
}

func (l *Lexer) skipWhitespaceAndComments() {
	for {
		r := l.peek()
		if r == 0 {
			return
		}
		if unicode.IsSpace(r) {
			l.next()
			continue
		}
		// Semicolon starts comment to end of line in OPS5
		if r == ';' {
			for l.peek() != 0 && l.peek() != '\n' {
				l.next()
			}
			continue
		}
		break
	}
}

// NextToken returns the next lexical token.
func (l *Lexer) NextToken() (Token, error) {
	l.skipWhitespaceAndComments()

	startLine := l.line
	startCol := l.col
	r := l.peek()

	if r == 0 {
		return Token{Type: TokenEOF, Value: "", Line: startLine, Col: startCol}, nil
	}

	if r == '(' {
		l.next()
		return Token{Type: TokenLParen, Value: "(", Line: startLine, Col: startCol}, nil
	}
	if r == ')' {
		l.next()
		return Token{Type: TokenRParen, Value: ")", Line: startLine, Col: startCol}, nil
	}
	if r == '{' {
		l.next()
		return Token{Type: TokenLBrace, Value: "{", Line: startLine, Col: startCol}, nil
	}
	if r == '}' {
		l.next()
		return Token{Type: TokenRBrace, Value: "}", Line: startLine, Col: startCol}, nil
	}
	if r == '[' {
		l.next()
		return Token{Type: TokenLBracket, Value: "[", Line: startLine, Col: startCol}, nil
	}
	if r == ']' {
		l.next()
		return Token{Type: TokenRBracket, Value: "]", Line: startLine, Col: startCol}, nil
	}

	// Arrow -->
	if r == '-' && l.cursor+2 < len(l.src) && l.src[l.cursor+1] == '-' && l.src[l.cursor+2] == '>' {
		l.next()
		l.next()
		l.next()
		return Token{Type: TokenArrow, Value: "-->", Line: startLine, Col: startCol}, nil
	}

	// Attribute ^foo
	if r == '^' {
		l.next()
		var b strings.Builder
		for {
			c := l.peek()
			if c == 0 || unicode.IsSpace(c) || c == '(' || c == ')' || c == '{' || c == '}' || c == '[' || c == ']' || c == '^' {
				break
			}
			b.WriteRune(l.next())
		}
		return Token{Type: TokenAttribute, Value: b.String(), Line: startLine, Col: startCol}, nil
	}

	// Variable <foo>, Operator (<>, <=, <), or Disjunction <<
	if r == '<' {
		l.next()
		if l.peek() == '<' {
			l.next()
			return Token{Type: TokenLDisj, Value: "<<", Line: startLine, Col: startCol}, nil
		}
		if l.peek() == '>' {
			l.next()
			return Token{Type: TokenOperator, Value: "<>", Line: startLine, Col: startCol}, nil
		}
		if l.peek() == '=' {
			l.next()
			return Token{Type: TokenOperator, Value: "<=", Line: startLine, Col: startCol}, nil
		}
		if unicode.IsSpace(l.peek()) || l.peek() == 0 || l.peek() == ')' || l.peek() == '}' || l.peek() == ']' {
			return Token{Type: TokenOperator, Value: "<", Line: startLine, Col: startCol}, nil
		}

		// It's a variable <var>
		var b strings.Builder
		for {
			c := l.peek()
			if c == 0 || c == '>' {
				break
			}
			b.WriteRune(l.next())
		}
		if l.peek() == '>' {
			l.next()
		}
		return Token{Type: TokenVariable, Value: b.String(), Line: startLine, Col: startCol}, nil
	}

	// Relational operators >, >=, Disjunction >>, =, !=
	if r == '>' {
		l.next()
		if l.peek() == '>' {
			l.next()
			return Token{Type: TokenRDisj, Value: ">>", Line: startLine, Col: startCol}, nil
		}
		if l.peek() == '=' {
			l.next()
			return Token{Type: TokenOperator, Value: ">=", Line: startLine, Col: startCol}, nil
		}
		return Token{Type: TokenOperator, Value: ">", Line: startLine, Col: startCol}, nil
	}
	if r == '=' {
		l.next()
		return Token{Type: TokenOperator, Value: "=", Line: startLine, Col: startCol}, nil
	}
	if r == '!' && l.cursor+1 < len(l.src) && l.src[l.cursor+1] == '=' {
		l.next()
		l.next()
		return Token{Type: TokenOperator, Value: "!=", Line: startLine, Col: startCol}, nil
	}

	// Negation '-' standalone (e.g. -(...) or - (...))
	if r == '-' && (l.cursor+1 < len(l.src) && (l.src[l.cursor+1] == '(' || unicode.IsSpace(l.src[l.cursor+1]))) {
		l.next()
		return Token{Type: TokenNegation, Value: "-", Line: startLine, Col: startCol}, nil
	}

	// String literal "..."
	if r == '"' {
		l.next()
		var b strings.Builder
		for {
			c := l.next()
			if c == 0 {
				return Token{}, fmt.Errorf("unterminated string at line %d, col %d", startLine, startCol)
			}
			if c == '"' {
				break
			}
			if c == '\\' {
				esc := l.next()
				switch esc {
				case 'n':
					b.WriteRune('\n')
				case 't':
					b.WriteRune('\t')
				default:
					b.WriteRune(esc)
				}
				continue
			}
			b.WriteRune(c)
		}
		return Token{Type: TokenString, Value: b.String(), Line: startLine, Col: startCol}, nil
	}

	// Vertical bar symbol literal |...|
	if r == '|' {
		l.next()
		var b strings.Builder
		for {
			c := l.next()
			if c == 0 {
				return Token{}, fmt.Errorf("unterminated vertical bar symbol at line %d, col %d", startLine, startCol)
			}
			if c == '|' {
				break
			}
			b.WriteRune(c)
		}
		return Token{Type: TokenSymbol, Value: b.String(), Line: startLine, Col: startCol}, nil
	}

	// Atom (number, symbol, or identifier)
	var b strings.Builder
	for {
		c := l.peek()
		if c == 0 || unicode.IsSpace(c) || c == '(' || c == ')' || c == '{' || c == '}' || c == '[' || c == ']' || c == '^' || c == ';' {
			break
		}
		b.WriteRune(l.next())
	}
	val := b.String()

	// Check if number
	if isNumber(val) {
		return Token{Type: TokenNumber, Value: val, Line: startLine, Col: startCol}, nil
	}

	return Token{Type: TokenSymbol, Value: val, Line: startLine, Col: startCol}, nil
}

func isNumber(s string) bool {
	if s == "" || s == "-" || s == "+" {
		return false
	}
	hasDigits := false
	for i, r := range s {
		if (r == '-' || r == '+') && i == 0 {
			continue
		}
		if unicode.IsDigit(r) {
			hasDigits = true
			continue
		}
		if r == '.' || r == 'e' || r == 'E' {
			continue
		}
		return false
	}
	return hasDigits
}

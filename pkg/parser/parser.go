package parser

import (
	"fmt"
	"strconv"
	"strings"

	"ops5/pkg/model"
)

// Parser parses OPS5 productions and commands into model objects.
type Parser struct {
	lexer   *Lexer
	current Token
	peek    Token
}

// NewParser creates a new Parser for the input string.
func NewParser(input string) (*Parser, error) {
	l := NewLexer(input)
	p := &Parser{lexer: l}

	tok1, err := l.NextToken()
	if err != nil {
		return nil, err
	}
	tok2, err := l.NextToken()
	if err != nil {
		return nil, err
	}
	p.current = tok1
	p.peek = tok2
	return p, nil
}

func (p *Parser) advance() error {
	p.current = p.peek
	tok, err := p.lexer.NextToken()
	if err != nil {
		return err
	}
	p.peek = tok
	return nil
}

func (p *Parser) expect(typ TokenType) (Token, error) {
	if p.current.Type != typ {
		return Token{}, fmt.Errorf("expected %s, got %s (%q) at line %d, col %d",
			typ, p.current.Type, p.current.Value, p.current.Line, p.current.Col)
	}
	tok := p.current
	if err := p.advance(); err != nil {
		return Token{}, err
	}
	return tok, nil
}

// ParseValue converts a token to a model.Value.
func TokenToValue(tok Token) model.Value {
	switch tok.Type {
	case TokenVariable:
		return model.NewVariable(tok.Value)
	case TokenString:
		return model.NewString(tok.Value)
	case TokenNumber:
		if n, err := strconv.ParseInt(tok.Value, 10, 64); err == nil {
			return model.NewInt(n)
		}
		if f, err := strconv.ParseFloat(tok.Value, 64); err == nil {
			return model.NewFloat(f)
		}
		return model.NewSymbol(tok.Value)
	default:
		return model.NewSymbol(tok.Value)
	}
}

// ParseRule parses a single (p rule-name ... --> ...) production.
func (p *Parser) ParseRule() (*model.Rule, error) {
	if _, err := p.expect(TokenLParen); err != nil {
		return nil, err
	}

	pToken, err := p.expect(TokenSymbol)
	if err != nil || strings.ToLower(pToken.Value) != "p" {
		return nil, fmt.Errorf("expected 'p' starting production at line %d", pToken.Line)
	}

	nameTok := p.current
	if nameTok.Type != TokenSymbol && nameTok.Type != TokenString {
		return nil, fmt.Errorf("expected rule name at line %d, got %s", nameTok.Line, nameTok.Value)
	}
	if err := p.advance(); err != nil {
		return nil, err
	}

	rule := model.NewRule(nameTok.Value)

	// Parse LHS condition elements until TokenArrow (-->)
	for p.current.Type != TokenArrow && p.current.Type != TokenEOF {
		ce, err := p.parseConditionElement()
		if err != nil {
			return nil, err
		}
		rule.AddCondition(ce)
	}

	if _, err := p.expect(TokenArrow); err != nil {
		return nil, err
	}

	// Parse RHS actions until closing ')'
	for p.current.Type != TokenRParen && p.current.Type != TokenEOF {
		action, err := p.parseAction()
		if err != nil {
			return nil, err
		}
		rule.AddAction(action)
	}

	if _, err := p.expect(TokenRParen); err != nil {
		return nil, err
	}

	return rule, nil
}

func (p *Parser) parseConditionElement() (*model.ConditionElement, error) {
	elemVar := ""
	if p.current.Type == TokenVariable {
		elemVar = p.current.Value
		if err := p.advance(); err != nil {
			return nil, err
		}
	}

	isNegated := false
	if p.current.Type == TokenNegation {
		isNegated = true
		if err := p.advance(); err != nil {
			return nil, err
		}
	}

	if _, err := p.expect(TokenLParen); err != nil {
		return nil, err
	}

	classTok, err := p.expect(TokenSymbol)
	if err != nil {
		return nil, fmt.Errorf("expected class name in condition element, got %v", err)
	}

	var ce *model.ConditionElement
	if isNegated {
		ce = model.NewNegativeCE(classTok.Value)
	} else {
		ce = model.NewPositiveCE(classTok.Value)
	}
	if elemVar != "" {
		ce.WithElementVariable(elemVar)
	}

	// Parse attribute tests
	for p.current.Type == TokenAttribute {
		attrName := p.current.Value
		if err := p.advance(); err != nil {
			return nil, err
		}

		// One or more constraints on this attribute
		for p.current.Type != TokenAttribute && p.current.Type != TokenRParen && p.current.Type != TokenEOF {
			op := model.OpEqual
			if p.current.Type == TokenOperator {
				op = model.ParseOperator(p.current.Value)
				if err := p.advance(); err != nil {
					return nil, err
				}
			}

			val := TokenToValue(p.current)
			if err := p.advance(); err != nil {
				return nil, err
			}

			ce.AddTest(attrName, op, val)
		}
	}

	if _, err := p.expect(TokenRParen); err != nil {
		return nil, err
	}

	return ce, nil
}

func (p *Parser) parseAction() (model.Action, error) {
	if _, err := p.expect(TokenLParen); err != nil {
		return nil, err
	}

	verbTok, err := p.expect(TokenSymbol)
	if err != nil {
		return nil, fmt.Errorf("expected action verb, got %v", err)
	}

	verb := strings.ToLower(verbTok.Value)
	switch verb {
	case "make":
		classTok, err := p.expect(TokenSymbol)
		if err != nil {
			return nil, fmt.Errorf("expected class in make action: %v", err)
		}
		attrs := make(map[string]model.Value)
		for p.current.Type == TokenAttribute {
			attrName := p.current.Value
			if err := p.advance(); err != nil {
				return nil, err
			}
			val := TokenToValue(p.current)
			if err := p.advance(); err != nil {
				return nil, err
			}
			attrs[attrName] = val
		}
		if _, err := p.expect(TokenRParen); err != nil {
			return nil, err
		}
		return model.MakeAction{Class: classTok.Value, Attributes: attrs}, nil

	case "modify":
		var targetVar string
		var targetIdx int
		if p.current.Type == TokenVariable {
			targetVar = p.current.Value
			p.advance()
		} else if p.current.Type == TokenNumber {
			idx, _ := strconv.Atoi(p.current.Value)
			targetIdx = idx
			p.advance()
		} else {
			return nil, fmt.Errorf("expected target variable or index in modify action, got %s", p.current.Value)
		}

		attrs := make(map[string]model.Value)
		for p.current.Type == TokenAttribute {
			attrName := p.current.Value
			if err := p.advance(); err != nil {
				return nil, err
			}
			val := TokenToValue(p.current)
			if err := p.advance(); err != nil {
				return nil, err
			}
			attrs[attrName] = val
		}
		if _, err := p.expect(TokenRParen); err != nil {
			return nil, err
		}
		return model.ModifyAction{
			TargetElementVar: targetVar,
			TargetIndex:      targetIdx,
			Attributes:       attrs,
		}, nil

	case "remove":
		var targetVar string
		var targetIdx int
		if p.current.Type == TokenVariable {
			targetVar = p.current.Value
			p.advance()
		} else if p.current.Type == TokenNumber {
			idx, _ := strconv.Atoi(p.current.Value)
			targetIdx = idx
			p.advance()
		} else {
			return nil, fmt.Errorf("expected target variable or index in remove action, got %s", p.current.Value)
		}
		if _, err := p.expect(TokenRParen); err != nil {
			return nil, err
		}
		return model.RemoveAction{
			TargetElementVar: targetVar,
			TargetIndex:      targetIdx,
		}, nil

	case "write":
		var items []model.Value
		for p.current.Type != TokenRParen && p.current.Type != TokenEOF {
			items = append(items, TokenToValue(p.current))
			p.advance()
		}
		if _, err := p.expect(TokenRParen); err != nil {
			return nil, err
		}
		return model.WriteAction{Items: items}, nil

	case "halt":
		if _, err := p.expect(TokenRParen); err != nil {
			return nil, err
		}
		return model.HaltAction{}, nil

	default:
		// Drain remaining tokens in unknown action
		for p.current.Type != TokenRParen && p.current.Type != TokenEOF {
			p.advance()
		}
		if _, err := p.expect(TokenRParen); err != nil {
			return nil, err
		}
		return model.CustomAction{Name: verb}, nil
	}
}

// ParseMake parses a standalone (make class ^attr val ...) statement.
func (p *Parser) ParseMake() (string, map[string]model.Value, error) {
	if _, err := p.expect(TokenLParen); err != nil {
		return "", nil, err
	}
	verbTok, err := p.expect(TokenSymbol)
	if err != nil || strings.ToLower(verbTok.Value) != "make" {
		return "", nil, fmt.Errorf("expected 'make', got %v", verbTok.Value)
	}

	classTok, err := p.expect(TokenSymbol)
	if err != nil {
		return "", nil, err
	}

	attrs := make(map[string]model.Value)
	for p.current.Type == TokenAttribute {
		attrName := p.current.Value
		if err := p.advance(); err != nil {
			return "", nil, err
		}
		val := TokenToValue(p.current)
		if err := p.advance(); err != nil {
			return "", nil, err
		}
		attrs[attrName] = val
	}

	if _, err := p.expect(TokenRParen); err != nil {
		return "", nil, err
	}

	return classTok.Value, attrs, nil
}

// ParseRules parses all rules from an OPS5 source string.
func ParseRules(source string) ([]*model.Rule, error) {
	p, err := NewParser(source)
	if err != nil {
		return nil, err
	}

	var rules []*model.Rule
	for p.current.Type != TokenEOF {
		rule, err := p.ParseRule()
		if err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}

	return rules, nil
}

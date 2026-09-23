package parser

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/graemenewlands/ops5/pkg/model"
)

// Parser parses OPS5 productions and commands into model objects.
type Parser struct {
	lexer       *Lexer
	current     Token
	peek        Token
	schemas     map[string]*model.ClassSchema
	vectorAttrs map[string]bool
}

// NewParser creates a new Parser for the input string.
func NewParser(input string) (*Parser, error) {
	l := NewLexer(input)
	p := &Parser{
		lexer:       l,
		schemas:     make(map[string]*model.ClassSchema),
		vectorAttrs: make(map[string]bool),
	}

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

// RegisterVectorAttribute declares an attribute name as a vector-attribute.
func (p *Parser) RegisterVectorAttribute(attr string) {
	norm := model.NormalizeAttribute(attr)
	if norm == "" {
		return
	}
	p.vectorAttrs[norm] = true
	for _, s := range p.schemas {
		if s.HasAttribute(norm) {
			s.SetVectorAttribute(norm, true)
		}
	}
}

// IsVectorAttribute returns true if the attribute is declared as a vector-attribute.
func (p *Parser) IsVectorAttribute(attr string) bool {
	return p.vectorAttrs[model.NormalizeAttribute(attr)]
}

// VectorAttributes returns a copy of declared vector attribute names.
func (p *Parser) VectorAttributes() []string {
	var res []string
	for a := range p.vectorAttrs {
		res = append(res, a)
	}
	return res
}

// RegisterSchema registers a class schema with the parser for positional attribute resolution.
func (p *Parser) RegisterSchema(schema *model.ClassSchema) {
	if schema != nil {
		for a := range p.vectorAttrs {
			if schema.HasAttribute(a) {
				schema.SetVectorAttribute(a, true)
			}
		}
		p.schemas[schema.Class] = schema
	}
}

func (p *Parser) getSchema(class string) *model.ClassSchema {
	return p.schemas[strings.ToLower(class)]
}

// Schemas returns all registered class schemas in the parser.
func (p *Parser) Schemas() []*model.ClassSchema {
	res := make([]*model.ClassSchema, 0, len(p.schemas))
	for _, s := range p.schemas {
		res = append(res, s)
	}
	return res
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
	case TokenSymbol:
		lower := strings.ToLower(tok.Value)
		if lower == "true" {
			return model.NewBoolean(true)
		}
		if lower == "false" {
			return model.NewBoolean(false)
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

	// Parse optional rule properties / documentation (e.g. [salience N], (salience N), "docstring")
	if err := p.parseRuleProperties(rule); err != nil {
		return nil, err
	}

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

func (p *Parser) parseInteger() (int, error) {
	sign := 1
	if p.current.Type == TokenNegation || (p.current.Type == TokenSymbol && p.current.Value == "-") {
		sign = -1
		if err := p.advance(); err != nil {
			return 0, err
		}
	} else if p.current.Type == TokenSymbol && p.current.Value == "+" {
		if err := p.advance(); err != nil {
			return 0, err
		}
	}

	if p.current.Type != TokenNumber {
		return 0, fmt.Errorf("expected integer, got %s (%q) at line %d", p.current.Type, p.current.Value, p.current.Line)
	}

	val, err := strconv.ParseInt(p.current.Value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid integer %q at line %d: %w", p.current.Value, p.current.Line, err)
	}
	if err := p.advance(); err != nil {
		return 0, err
	}
	if val < 0 {
		return int(val), nil
	}
	return int(val) * sign, nil
}

func (p *Parser) parseRuleProperties(rule *model.Rule) error {
	for {
		if p.current.Type == TokenString {
			rule.Docstring = p.current.Value
			if err := p.advance(); err != nil {
				return err
			}
			continue
		}

		if p.current.Type == TokenLBracket {
			if err := p.advance(); err != nil {
				return err
			}
			propName := strings.ToLower(p.current.Value)
			propName = strings.TrimSuffix(propName, ":")
			if propName == "salience" {
				if err := p.advance(); err != nil {
					return err
				}
				if p.current.Type == TokenSymbol && (p.current.Value == ":" || p.current.Value == "=") {
					if err := p.advance(); err != nil {
						return err
					}
				}
				sal, err := p.parseInteger()
				if err != nil {
					return fmt.Errorf("invalid salience value at line %d: %w", p.current.Line, err)
				}
				rule.Salience = sal
			} else if propName == "no-reorder" || propName == "noreorder" || propName == "no_reorder" {
				rule.NoReorder = true
				if err := p.advance(); err != nil {
					return err
				}
			} else {
				return fmt.Errorf("unknown rule property [%s] at line %d", p.current.Value, p.current.Line)
			}
			if _, err := p.expect(TokenRBracket); err != nil {
				return err
			}
			continue
		}

		if p.current.Type == TokenLParen {
			if strings.EqualFold(p.peek.Value, "declare") {
				if err := p.advance(); err != nil { // consume '('
					return err
				}
				if err := p.advance(); err != nil { // consume 'declare'
					return err
				}
				for p.current.Type != TokenRParen && p.current.Type != TokenEOF {
					if _, err := p.expect(TokenLParen); err != nil {
						return err
					}
					decName := strings.ToLower(p.current.Value)
					decName = strings.TrimSuffix(decName, ":")
					if decName == "salience" {
						if err := p.advance(); err != nil {
							return err
						}
						if p.current.Type == TokenSymbol && (p.current.Value == ":" || p.current.Value == "=") {
							if err := p.advance(); err != nil {
								return err
							}
						}
						sal, err := p.parseInteger()
						if err != nil {
							return fmt.Errorf("invalid salience value at line %d: %w", p.current.Line, err)
						}
						rule.Salience = sal
					} else if decName == "no-reorder" || decName == "noreorder" || decName == "no_reorder" {
						rule.NoReorder = true
						if err := p.advance(); err != nil {
							return err
						}
					} else {
						return fmt.Errorf("unknown declaration (%s) at line %d", p.current.Value, p.current.Line)
					}
					if _, err := p.expect(TokenRParen); err != nil {
						return err
					}
				}
				if _, err := p.expect(TokenRParen); err != nil {
					return err
				}
				continue
			}

			peekLower := strings.ToLower(p.peek.Value)
			peekClean := strings.TrimSuffix(peekLower, ":")
			if peekClean == "no-reorder" || peekClean == "noreorder" || peekClean == "no_reorder" {
				if err := p.advance(); err != nil { // consume '('
					return err
				}
				if err := p.advance(); err != nil { // consume 'no-reorder'
					return err
				}
				rule.NoReorder = true
				if _, err := p.expect(TokenRParen); err != nil {
					return err
				}
				continue
			}
			if peekClean == "salience" {
				lx := *p.lexer
				tok3, _ := lx.NextToken()
				if tok3.Type == TokenNumber || tok3.Type == TokenNegation || tok3.Value == ":" || tok3.Value == "=" || (tok3.Type == TokenSymbol && isNumber(tok3.Value)) {
					if err := p.advance(); err != nil { // consume '('
						return err
					}
					if err := p.advance(); err != nil { // consume 'salience'
						return err
					}
					if p.current.Type == TokenSymbol && (p.current.Value == ":" || p.current.Value == "=") {
						if err := p.advance(); err != nil {
							return err
						}
					}
					sal, err := p.parseInteger()
					if err != nil {
						return fmt.Errorf("invalid salience value at line %d: %w", p.current.Line, err)
					}
					rule.Salience = sal
					if _, err := p.expect(TokenRParen); err != nil {
						return err
					}
					continue
				}
			}
		}

		break
	}
	return nil
}

func (p *Parser) isAttributeToken(tok Token, schema *model.ClassSchema) bool {
	if tok.Type == TokenAttribute {
		return true
	}
	if tok.Type == TokenSymbol {
		norm := model.NormalizeAttribute(tok.Value)
		if schema != nil && schema.HasAttribute(norm) {
			return true
		}
	}
	return false
}

func (p *Parser) parseConditionElement() (*model.ConditionElement, error) {
	isBraced := false
	if p.current.Type == TokenLBrace {
		isBraced = true
		if err := p.advance(); err != nil {
			return nil, err
		}
	}

	elemVar := ""
	for p.current.Type == TokenVariable {
		if elemVar == "" {
			elemVar = p.current.Value
		}
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

	if isNegated && (p.current.Type == TokenLParen || p.current.Type == TokenLBrace) {
		if elemVar != "" {
			return nil, fmt.Errorf("element variables cannot be bound to negated conjunction at line %d", p.current.Line)
		}
		return p.parseNccCondition(isBraced)
	}

	classTok, err := p.expect(TokenSymbol)
	if err != nil {
		return nil, fmt.Errorf("expected class name in condition element, got %v", err)
	}

	if strings.EqualFold(classTok.Value, "ncc") {
		if elemVar != "" {
			return nil, fmt.Errorf("element variables cannot be bound to negated conjunction at line %d", classTok.Line)
		}
		return p.parseNccCondition(isBraced)
	}

	if strings.EqualFold(classTok.Value, "test") {
		if isNegated {
			return nil, fmt.Errorf("test condition element cannot be negated at line %d", classTok.Line)
		}
		if elemVar != "" {
			return nil, fmt.Errorf("element variables cannot be bound to test condition elements at line %d", classTok.Line)
		}
		return p.parseTestCondition(isBraced)
	}

	if strings.EqualFold(classTok.Value, "exists") {
		if isNegated {
			return nil, fmt.Errorf("exists condition element cannot be negated at line %d; use standard negated condition -(...) instead", classTok.Line)
		}
		if elemVar != "" {
			return nil, fmt.Errorf("element variables cannot be bound to existential condition elements at line %d", classTok.Line)
		}
		return p.parseExistentialCondition(isBraced)
	}

	if strings.EqualFold(classTok.Value, "accumulate") || strings.EqualFold(classTok.Value, "acc") {
		if isNegated {
			return nil, fmt.Errorf("accumulate condition element cannot be negated at line %d", classTok.Line)
		}
		if elemVar != "" {
			return nil, fmt.Errorf("element variables cannot be bound to accumulate condition elements at line %d", classTok.Line)
		}
		return p.parseAccumulateCondition(isBraced)
	}

	var ce *model.ConditionElement
	if isNegated {
		ce = model.NewNegativeCE(classTok.Value)
	} else {
		ce = model.NewPositiveCE(classTok.Value)
	}

	schema := p.getSchema(classTok.Value)
	var orderedAttrs []string
	posIndex := 0

	// Parse attribute tests (both named ^attr and positional)
	for p.current.Type != TokenRParen && p.current.Type != TokenEOF {
		if p.isAttributeToken(p.current, schema) {
			attrName := model.NormalizeAttribute(p.current.Value)
			orderedAttrs = append(orderedAttrs, attrName)
			if schema != nil {
				schema.AddAttribute(attrName)
			}
			if err := p.advance(); err != nil {
				return nil, err
			}

			// One or more constraints on this attribute
			for !p.isAttributeToken(p.current, schema) && p.current.Type != TokenRParen && p.current.Type != TokenEOF {
				if p.current.Type == TokenLDisj {
					if err := p.advance(); err != nil {
						return nil, err
					}
					var disj []model.TestConstraint
					for p.current.Type != TokenRDisj && p.current.Type != TokenRParen && p.current.Type != TokenEOF {
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
						disj = append(disj, model.TestConstraint{Op: op, Value: val})
					}
					if _, err := p.expect(TokenRDisj); err != nil {
						return nil, fmt.Errorf("expected '>>' closing attribute disjunction on ^%s: %w", attrName, err)
					}
					ce.AddDisjunctionTest(attrName, disj)
					continue
				}

				if p.current.Type == TokenLBrace {
					if err := p.advance(); err != nil {
						return nil, err
					}
					for p.current.Type != TokenRBrace && p.current.Type != TokenRParen && p.current.Type != TokenEOF {
						if p.current.Type == TokenLDisj {
							if err := p.advance(); err != nil {
								return nil, err
							}
							var disj []model.TestConstraint
							for p.current.Type != TokenRDisj && p.current.Type != TokenRBrace && p.current.Type != TokenRParen && p.current.Type != TokenEOF {
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
								disj = append(disj, model.TestConstraint{Op: op, Value: val})
							}
							if _, err := p.expect(TokenRDisj); err != nil {
								return nil, fmt.Errorf("expected '>>' closing attribute disjunction in brace on ^%s: %w", attrName, err)
							}
							ce.AddDisjunctionTest(attrName, disj)
							continue
						}

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
					if _, err := p.expect(TokenRBrace); err != nil {
						return nil, fmt.Errorf("expected '}' closing attribute conjunction on ^%s: %w", attrName, err)
					}
					continue
				}

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
		} else {
			// Positional test in condition element
			attrName := fmt.Sprintf("attr%d", posIndex+1)
			if schema != nil && posIndex < len(schema.Attributes) {
				attrName = schema.Attributes[posIndex]
			}
			posIndex++

			if p.current.Type == TokenLDisj {
				if err := p.advance(); err != nil {
					return nil, err
				}
				var disj []model.TestConstraint
				for p.current.Type != TokenRDisj && p.current.Type != TokenRParen && p.current.Type != TokenEOF {
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
					disj = append(disj, model.TestConstraint{Op: op, Value: val})
				}
				if _, err := p.expect(TokenRDisj); err != nil {
					return nil, fmt.Errorf("expected '>>' closing positional disjunction on %s: %w", attrName, err)
				}
				ce.AddDisjunctionTest(attrName, disj)
			} else if p.current.Type == TokenLBrace {
				if err := p.advance(); err != nil {
					return nil, err
				}
				for p.current.Type != TokenRBrace && p.current.Type != TokenRParen && p.current.Type != TokenEOF {
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
				if _, err := p.expect(TokenRBrace); err != nil {
					return nil, fmt.Errorf("expected '}' closing positional conjunction on %s: %w", attrName, err)
				}
			} else {
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
	}

	if _, err := p.expect(TokenRParen); err != nil {
		return nil, err
	}

	if schema == nil && len(orderedAttrs) > 0 {
		schema = model.NewClassSchema(classTok.Value, orderedAttrs)
		p.RegisterSchema(schema)
	}

	if isBraced {
		for p.current.Type == TokenVariable {
			if elemVar == "" {
				elemVar = p.current.Value
			}
			if err := p.advance(); err != nil {
				return nil, err
			}
		}
		if _, err := p.expect(TokenRBrace); err != nil {
			return nil, fmt.Errorf("expected '}' closing condition element conjunction: %w", err)
		}
	}

	if elemVar != "" {
		ce.WithElementVariable(elemVar)
	}

	return ce, nil
}

func (p *Parser) parseTestCondition(isBraced bool) (*model.ConditionElement, error) {
	var comparisons []model.EvalComparison

	for p.current.Type != TokenRParen && p.current.Type != TokenEOF {
		cmp, err := p.parseTestComparison()
		if err != nil {
			return nil, err
		}
		comparisons = append(comparisons, cmp)
	}

	if _, err := p.expect(TokenRParen); err != nil {
		return nil, err
	}

	if isBraced {
		if _, err := p.expect(TokenRBrace); err != nil {
			return nil, fmt.Errorf("expected '}' closing condition element conjunction: %w", err)
		}
	}

	if len(comparisons) == 0 {
		return nil, fmt.Errorf("empty test condition element")
	}

	return model.NewTestCE(model.NewEvalTest(comparisons...)), nil
}

func (p *Parser) parseExistentialCondition(isBraced bool) (*model.ConditionElement, error) {
	hasInnerParen := false
	if p.current.Type == TokenLParen {
		hasInnerParen = true
		if err := p.advance(); err != nil {
			return nil, err
		}
	}

	classTok, err := p.expect(TokenSymbol)
	if err != nil {
		return nil, fmt.Errorf("expected class name in existential condition at line %d, got %v", p.current.Line, err)
	}

	ce := model.NewExistentialCE(classTok.Value)
	schema := p.getSchema(classTok.Value)
	var orderedAttrs []string
	posIndex := 0

	for p.current.Type != TokenRParen && p.current.Type != TokenEOF {
		if p.isAttributeToken(p.current, schema) {
			attrName := model.NormalizeAttribute(p.current.Value)
			orderedAttrs = append(orderedAttrs, attrName)
			if schema != nil {
				schema.AddAttribute(attrName)
			}
			if err := p.advance(); err != nil {
				return nil, err
			}

			for !p.isAttributeToken(p.current, schema) && p.current.Type != TokenRParen && p.current.Type != TokenEOF {
				if p.current.Type == TokenLBrace {
					if err := p.advance(); err != nil {
						return nil, err
					}
					for p.current.Type != TokenRBrace && p.current.Type != TokenRParen && p.current.Type != TokenEOF {
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
					if _, err := p.expect(TokenRBrace); err != nil {
						return nil, fmt.Errorf("expected '}' closing attribute conjunction on ^%s: %w", attrName, err)
					}
					continue
				}

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
		} else {
			attrName := fmt.Sprintf("attr%d", posIndex+1)
			if schema != nil && posIndex < len(schema.Attributes) {
				attrName = schema.Attributes[posIndex]
			}
			posIndex++

			if p.current.Type == TokenLBrace {
				if err := p.advance(); err != nil {
					return nil, err
				}
				for p.current.Type != TokenRBrace && p.current.Type != TokenRParen && p.current.Type != TokenEOF {
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
				if _, err := p.expect(TokenRBrace); err != nil {
					return nil, fmt.Errorf("expected '}' closing positional conjunction on %s: %w", attrName, err)
				}
			} else {
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
	}

	if hasInnerParen {
		if _, err := p.expect(TokenRParen); err != nil {
			return nil, fmt.Errorf("expected ')' closing inner condition in exists: %w", err)
		}
	}

	if _, err := p.expect(TokenRParen); err != nil {
		return nil, fmt.Errorf("expected ')' closing (exists ...): %w", err)
	}

	if schema == nil && len(orderedAttrs) > 0 {
		schema = model.NewClassSchema(classTok.Value, orderedAttrs)
		p.RegisterSchema(schema)
	}

	if isBraced {
		if _, err := p.expect(TokenRBrace); err != nil {
			return nil, fmt.Errorf("expected '}' closing condition element conjunction: %w", err)
		}
	}

	return ce, nil
}

func isAccumulateOpToken(tok Token) bool {
	if tok.Type != TokenSymbol {
		return false
	}
	v := strings.ToLower(strings.TrimPrefix(tok.Value, ":"))
	switch v {
	case "count", "sum", "avg", "average", "min", "max", "collect":
		return true
	}
	return false
}

func (p *Parser) parseAccumulateCondition(isBraced bool) (*model.ConditionElement, error) {
	hasInnerParen := false
	if p.current.Type == TokenLParen {
		hasInnerParen = true
		if err := p.advance(); err != nil {
			return nil, err
		}
	}

	classTok, err := p.expect(TokenSymbol)
	if err != nil {
		return nil, fmt.Errorf("expected class name in accumulate condition at line %d, got %v", p.current.Line, err)
	}

	schema := p.getSchema(classTok.Value)
	var orderedAttrs []string
	posIndex := 0

	tempCE := model.NewPositiveCE(classTok.Value)

	for p.current.Type != TokenRParen && p.current.Type != TokenEOF {
		if !hasInnerParen && isAccumulateOpToken(p.current) {
			break
		}

		if p.isAttributeToken(p.current, schema) {
			attrName := model.NormalizeAttribute(p.current.Value)
			orderedAttrs = append(orderedAttrs, attrName)
			if schema != nil {
				schema.AddAttribute(attrName)
			}
			if err := p.advance(); err != nil {
				return nil, err
			}

			for !p.isAttributeToken(p.current, schema) && p.current.Type != TokenRParen && p.current.Type != TokenEOF {
				if !hasInnerParen && isAccumulateOpToken(p.current) {
					break
				}
				if p.current.Type == TokenLBrace {
					if err := p.advance(); err != nil {
						return nil, err
					}
					for p.current.Type != TokenRBrace && p.current.Type != TokenRParen && p.current.Type != TokenEOF {
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
						tempCE.AddTest(attrName, op, val)
					}
					if _, err := p.expect(TokenRBrace); err != nil {
						return nil, fmt.Errorf("expected '}' closing attribute conjunction on ^%s: %w", attrName, err)
					}
					continue
				}

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
				tempCE.AddTest(attrName, op, val)
			}
		} else {
			// Positional test
			attrName := fmt.Sprintf("attr%d", posIndex+1)
			if schema != nil && posIndex < len(schema.Attributes) {
				attrName = schema.Attributes[posIndex]
			}
			posIndex++

			if p.current.Type == TokenLBrace {
				if err := p.advance(); err != nil {
					return nil, err
				}
				for p.current.Type != TokenRBrace && p.current.Type != TokenRParen && p.current.Type != TokenEOF {
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
					tempCE.AddTest(attrName, op, val)
				}
				if _, err := p.expect(TokenRBrace); err != nil {
					return nil, fmt.Errorf("expected '}' closing positional conjunction on %s: %w", attrName, err)
				}
			} else {
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
				tempCE.AddTest(attrName, op, val)
			}
		}
	}

	if hasInnerParen {
		if _, err := p.expect(TokenRParen); err != nil {
			return nil, fmt.Errorf("expected ')' closing inner condition in accumulate: %w", err)
		}
	}

	if schema == nil && len(orderedAttrs) > 0 {
		schema = model.NewClassSchema(classTok.Value, orderedAttrs)
		p.RegisterSchema(schema)
	}

	// Parse accumulate operation: e.g. :sum, sum, :count, etc.
	if !isAccumulateOpToken(p.current) {
		return nil, fmt.Errorf("expected accumulate operation (e.g. :count, :sum, :min, :max, :avg, :collect) at line %d, got %s", p.current.Line, p.current.Value)
	}

	opName := p.current.Value
	accOp, err := model.ParseAccumulateOp(opName)
	if err != nil {
		return nil, fmt.Errorf("invalid accumulate operation '%s' at line %d: %w", opName, p.current.Line, err)
	}
	if err := p.advance(); err != nil {
		return nil, err
	}

	var target model.Value
	var resultVar string

	if accOp == model.AccCount {
		firstTok := p.current
		if err := p.advance(); err != nil {
			return nil, err
		}
		if p.current.Type == TokenRParen {
			// Single argument: result variable
			resultVar = strings.TrimPrefix(strings.TrimSuffix(firstTok.Value, ">"), "<")
		} else {
			// Two arguments: target, result variable
			target = TokenToValue(firstTok)
			resTok := p.current
			if err := p.advance(); err != nil {
				return nil, err
			}
			resultVar = strings.TrimPrefix(strings.TrimSuffix(resTok.Value, ">"), "<")
		}
	} else {
		// Target expression
		if p.current.Type == TokenLParen {
			cVal, err := p.parseCompute()
			if err != nil {
				return nil, fmt.Errorf("error parsing target compute expression in accumulate at line %d: %w", p.current.Line, err)
			}
			target = cVal
		} else {
			target = TokenToValue(p.current)
			if err := p.advance(); err != nil {
				return nil, err
			}
		}

		// Result variable
		resTok := p.current
		if resTok.Type != TokenVariable && resTok.Type != TokenSymbol {
			return nil, fmt.Errorf("expected result variable in accumulate at line %d, got %s", resTok.Line, resTok.Value)
		}
		resultVar = strings.TrimPrefix(strings.TrimSuffix(resTok.Value, ">"), "<")
		if err := p.advance(); err != nil {
			return nil, err
		}
	}

	if _, err := p.expect(TokenRParen); err != nil {
		return nil, fmt.Errorf("expected ')' closing (accumulate ...): %w", err)
	}

	if isBraced {
		if _, err := p.expect(TokenRBrace); err != nil {
			return nil, fmt.Errorf("expected '}' closing condition element conjunction: %w", err)
		}
	}

	spec := &model.AccumulateSpec{
		Op:        accOp,
		Target:    target,
		ResultVar: resultVar,
	}

	ce := model.NewAccumulateCE(classTok.Value, spec)
	ce.Tests = tempCE.Tests

	return ce, nil
}

func (p *Parser) parseNccCondition(isOuterBraced bool) (*model.ConditionElement, error) {
	isInnerBraced := false
	if p.current.Type == TokenLBrace {
		isInnerBraced = true
		if err := p.advance(); err != nil {
			return nil, err
		}
	}

	var subConditions []*model.ConditionElement

	for p.current.Type != TokenRParen && p.current.Type != TokenRBrace && p.current.Type != TokenEOF {
		subCE, err := p.parseConditionElement()
		if err != nil {
			return nil, fmt.Errorf("error parsing sub-condition in negated conjunction: %w", err)
		}
		subConditions = append(subConditions, subCE)
	}

	if isInnerBraced {
		if _, err := p.expect(TokenRBrace); err != nil {
			return nil, fmt.Errorf("expected '}' closing inner negated conjunction: %w", err)
		}
	}

	if _, err := p.expect(TokenRParen); err != nil {
		return nil, fmt.Errorf("expected ')' closing negated conjunction: %w", err)
	}

	if isOuterBraced {
		if _, err := p.expect(TokenRBrace); err != nil {
			return nil, fmt.Errorf("expected '}' closing outer conjunction: %w", err)
		}
	}

	if len(subConditions) == 0 {
		return nil, fmt.Errorf("empty negated conjunction at line %d", p.current.Line)
	}

	return model.NewNccCE(subConditions), nil
}

func (p *Parser) parseTestComparison() (model.EvalComparison, error) {
	hasParen := false
	if p.current.Type == TokenLParen {
		hasParen = true
		if err := p.advance(); err != nil {
			return model.EvalComparison{}, err
		}
	}

	// Check if this is a prefix operator like (> <x> <y>)
	if op, ok := isRelationalOp(p.current); ok {
		if err := p.advance(); err != nil {
			return model.EvalComparison{}, err
		}
		left, err := p.parseTestOperand()
		if err != nil {
			return model.EvalComparison{}, err
		}
		right, err := p.parseTestOperand()
		if err != nil {
			return model.EvalComparison{}, err
		}
		if hasParen {
			if _, err := p.expect(TokenRParen); err != nil {
				return model.EvalComparison{}, err
			}
		}
		return model.EvalComparison{Left: left, Op: op, Right: right, HasRight: true}, nil
	}

	// Check if this is (compute <x> + <y> > 100) where compute is directly inside without inner parens
	if strings.EqualFold(p.current.Value, "compute") {
		if err := p.advance(); err != nil {
			return model.EvalComparison{}, err
		}
		var operands []model.Value
		var operators []model.ComputeOp

		for p.current.Type != TokenRParen && p.current.Type != TokenEOF {
			// Check if at start and unary minus
			if len(operands) == 0 {
				if op, ok := isArithmeticOp(p.current); ok && op == model.ComputeOpSub {
					operands = append(operands, model.NewInt(0))
					operators = append(operators, model.ComputeOpSub)
					if err := p.advance(); err != nil {
						return model.EvalComparison{}, err
					}
					continue
				}
			}

			// Check if we hit a relational operator
			if relOp, ok := isRelationalOp(p.current); ok {
				if len(operands) == 0 {
					return model.EvalComparison{}, fmt.Errorf("unexpected relational operator %s in compute at line %d", p.current.Value, p.current.Line)
				}
				if err := p.advance(); err != nil {
					return model.EvalComparison{}, err
				}
				right, err := p.parseTestOperand()
				if err != nil {
					return model.EvalComparison{}, err
				}
				if hasParen {
					if _, err := p.expect(TokenRParen); err != nil {
						return model.EvalComparison{}, err
					}
				}
				return model.EvalComparison{
					Left:     model.NewCompute(operands, operators),
					Op:       relOp,
					Right:    right,
					HasRight: true,
				}, nil
			}

			// Parse operand
			if p.isRHSFunction() {
				subFn, err := p.parseRHSFunction()
				if err != nil {
					return model.EvalComparison{}, err
				}
				operands = append(operands, subFn)
			} else if p.current.Type == TokenNumber || p.current.Type == TokenVariable || p.current.Type == TokenSymbol || p.current.Type == TokenString {
				operands = append(operands, TokenToValue(p.current))
				if err := p.advance(); err != nil {
					return model.EvalComparison{}, err
				}
			} else {
				return model.EvalComparison{}, fmt.Errorf("unexpected token %s in compute at line %d", p.current.Value, p.current.Line)
			}

			// Check if next is a relational op or rparen
			if relOp, ok := isRelationalOp(p.current); ok {
				if err := p.advance(); err != nil {
					return model.EvalComparison{}, err
				}
				right, err := p.parseTestOperand()
				if err != nil {
					return model.EvalComparison{}, err
				}
				if hasParen {
					if _, err := p.expect(TokenRParen); err != nil {
						return model.EvalComparison{}, err
					}
				}
				return model.EvalComparison{
					Left:     model.NewCompute(operands, operators),
					Op:       relOp,
					Right:    right,
					HasRight: true,
				}, nil
			}

			if p.current.Type == TokenRParen || p.current.Type == TokenEOF {
				break
			}

			// Must be an arithmetic operator (+, -, *, /, etc.)
			op, ok := isArithmeticOp(p.current)
			if !ok {
				return model.EvalComparison{}, fmt.Errorf("expected arithmetic operator in compute at line %d, got %s", p.current.Line, p.current.Value)
			}
			operators = append(operators, op)
			if err := p.advance(); err != nil {
				return model.EvalComparison{}, err
			}
		}

		if hasParen {
			if _, err := p.expect(TokenRParen); err != nil {
				return model.EvalComparison{}, err
			}
		}
		// Single compute expression evaluated for truthiness
		return model.EvalComparison{Left: model.NewCompute(operands, operators), HasRight: false}, nil
	}

	// Normal infix: <left> <op> <right> or unary <left>
	left, err := p.parseTestOperand()
	if err != nil {
		return model.EvalComparison{}, err
	}

	if hasParen && p.current.Type == TokenRParen {
		// Unary truthiness test: (<left>)
		if err := p.advance(); err != nil {
			return model.EvalComparison{}, err
		}
		return model.EvalComparison{Left: left, HasRight: false}, nil
	}

	// Must have relational operator
	relOp, ok := isRelationalOp(p.current)
	if !ok {
		if !hasParen && (p.current.Type == TokenRParen || p.current.Type == TokenEOF) {
			return model.EvalComparison{Left: left, HasRight: false}, nil
		}
		return model.EvalComparison{}, fmt.Errorf("expected relational operator in test comparison at line %d, got %s", p.current.Line, p.current.Value)
	}
	if err := p.advance(); err != nil {
		return model.EvalComparison{}, err
	}

	right, err := p.parseTestOperand()
	if err != nil {
		return model.EvalComparison{}, err
	}

	if hasParen {
		if _, err := p.expect(TokenRParen); err != nil {
			return model.EvalComparison{}, err
		}
	}

	return model.EvalComparison{Left: left, Op: relOp, Right: right, HasRight: true}, nil
}

func (p *Parser) parseTestOperand() (model.Value, error) {
	if p.current.Type == TokenLParen {
		if strings.EqualFold(p.peek.Value, "compute") {
			return p.parseCompute()
		}
		// Sub-expression in parens: could be (<val>) or ((compute ...))
		if err := p.advance(); err != nil {
			return model.NewInt(0), err
		}
		val, err := p.parseTestOperand()
		if err != nil {
			return model.NewInt(0), err
		}
		if _, err := p.expect(TokenRParen); err != nil {
			return model.NewInt(0), err
		}
		return val, nil
	}

	if p.current.Type == TokenNumber || p.current.Type == TokenVariable || p.current.Type == TokenSymbol || p.current.Type == TokenString {
		val := TokenToValue(p.current)
		if err := p.advance(); err != nil {
			return model.NewInt(0), err
		}
		return val, nil
	}

	return model.NewInt(0), fmt.Errorf("expected operand (number, variable, symbol, string, or compute) in test at line %d, got %s", p.current.Line, p.current.Value)
}

func isRelationalOp(tok Token) (model.Operator, bool) {
	if tok.Type == TokenOperator {
		return model.ParseOperator(tok.Value), true
	}
	if tok.Type == TokenSymbol {
		switch tok.Value {
		case "=", "<>", "!=", "<", "<=", ">", ">=":
			return model.ParseOperator(tok.Value), true
		}
	}
	return model.OpEqual, false
}

func isArithmeticOp(tok Token) (model.ComputeOp, bool) {
	if tok.Type == TokenNegation && tok.Value == "-" {
		return model.ComputeOpSub, true
	}
	if tok.Type == TokenSymbol || tok.Type == TokenOperator {
		switch tok.Value {
		case "+":
			return model.ComputeOpAdd, true
		case "-":
			return model.ComputeOpSub, true
		case "*":
			return model.ComputeOpMul, true
		case "/":
			return model.ComputeOpDiv, true
		case "//", "\\", "%":
			return model.ComputeOpMod, true
		}
	}
	return 0, false
}

// parseCompute parses a (compute ...) arithmetic expression.
func (p *Parser) parseCompute() (model.Value, error) {
	if _, err := p.expect(TokenLParen); err != nil {
		return model.NewInt(0), err
	}
	compTok, err := p.expect(TokenSymbol)
	if err != nil || strings.ToLower(compTok.Value) != "compute" {
		return model.NewInt(0), fmt.Errorf("expected 'compute' starting arithmetic expression, got %v", compTok.Value)
	}

	var operands []model.Value
	var operators []model.ComputeOp

	for p.current.Type != TokenRParen && p.current.Type != TokenEOF {
		// If at start and unary minus:
		if len(operands) == 0 {
			if op, ok := isArithmeticOp(p.current); ok && op == model.ComputeOpSub {
				operands = append(operands, model.NewInt(0))
				operators = append(operators, model.ComputeOpSub)
				if err := p.advance(); err != nil {
					return model.NewInt(0), err
				}
				continue
			}
		}

		// Parse operand
		if p.isRHSFunction() {
			subFn, err := p.parseRHSFunction()
			if err != nil {
				return model.NewInt(0), err
			}
			operands = append(operands, subFn)
		} else if p.current.Type == TokenNumber || p.current.Type == TokenVariable || p.current.Type == TokenSymbol {
			operands = append(operands, TokenToValue(p.current))
			if err := p.advance(); err != nil {
				return model.NewInt(0), err
			}
		} else {
			return model.NewInt(0), fmt.Errorf("expected number, variable, or function in compute expression at line %d, got %v", p.current.Line, p.current.Value)
		}

		if p.current.Type == TokenRParen || p.current.Type == TokenEOF {
			break
		}

		// Parse operator
		op, ok := isArithmeticOp(p.current)
		if !ok {
			return model.NewInt(0), fmt.Errorf("expected arithmetic operator (+, -, *, /, //) in compute expression at line %d, got %v", p.current.Line, p.current.Value)
		}
		operators = append(operators, op)
		if err := p.advance(); err != nil {
			return model.NewInt(0), err
		}
	}

	if _, err := p.expect(TokenRParen); err != nil {
		return model.NewInt(0), err
	}

	if len(operands) == 0 {
		return model.NewInt(0), fmt.Errorf("empty compute expression at line %d", compTok.Line)
	}

	return model.NewCompute(operands, operators), nil
}

func (p *Parser) isRHSFunction() bool {
	if p.current.Type != TokenLParen {
		return false
	}
	sub := strings.ToLower(p.peek.Value)
	return sub == "compute" || sub == "accept" || sub == "acceptline" || sub == "genatom" || sub == "litval" || sub == "substr"
}

func (p *Parser) parseRHSFunction() (model.Value, error) {
	sub := strings.ToLower(p.peek.Value)
	switch sub {
	case "compute":
		return p.parseCompute()
	case "accept":
		return p.parseAccept(false)
	case "acceptline":
		return p.parseAccept(true)
	case "genatom":
		return p.parseGenatom()
	case "litval":
		return p.parseLitval()
	case "substr":
		return p.ParseSubstr()
	default:
		return model.NewSymbol("nil"), fmt.Errorf("unknown RHS function: %s", sub)
	}
}

func (p *Parser) parseSubstrArg() (model.Value, error) {
	if p.isRHSFunction() {
		return p.parseRHSFunction()
	}
	if p.current.Type == TokenAttribute {
		attr := model.NormalizeAttribute(p.current.Value)
		if err := p.advance(); err != nil {
			return model.NewSymbol("nil"), err
		}
		return model.NewSymbol(attr), nil
	}
	if p.current.Type == TokenVariable || p.current.Type == TokenSymbol || p.current.Type == TokenNumber || p.current.Type == TokenString {
		val := TokenToValue(p.current)
		if err := p.advance(); err != nil {
			return model.NewSymbol("nil"), err
		}
		return val, nil
	}
	return model.NewSymbol("nil"), fmt.Errorf("unexpected token in substr at line %d: %s (%q)", p.current.Line, p.current.Type, p.current.Value)
}

// ParseSubstr parses a (substr elemRef start end) function call.
func (p *Parser) ParseSubstr() (model.Value, error) {
	if _, err := p.expect(TokenLParen); err != nil {
		return model.NewSymbol("nil"), err
	}
	verbTok, err := p.expect(TokenSymbol)
	if err != nil || !strings.EqualFold(verbTok.Value, "substr") {
		return model.NewSymbol("nil"), fmt.Errorf("expected 'substr', got %v", verbTok.Value)
	}

	elemRef, err := p.parseSubstrArg()
	if err != nil {
		return model.NewSymbol("nil"), fmt.Errorf("error parsing element reference in substr: %w", err)
	}

	start, err := p.parseSubstrArg()
	if err != nil {
		return model.NewSymbol("nil"), fmt.Errorf("error parsing start index in substr: %w", err)
	}

	end, err := p.parseSubstrArg()
	if err != nil {
		return model.NewSymbol("nil"), fmt.Errorf("error parsing end index in substr: %w", err)
	}

	if _, err := p.expect(TokenRParen); err != nil {
		return model.NewSymbol("nil"), fmt.Errorf("expected ')' closing substr at line %d: %w", verbTok.Line, err)
	}

	return model.NewSubstr(elemRef, start, end), nil
}

func (p *Parser) parseLitval() (model.Value, error) {
	if _, err := p.expect(TokenLParen); err != nil {
		return model.NewSymbol("nil"), err
	}
	verbTok, err := p.expect(TokenSymbol)
	if err != nil || strings.ToLower(verbTok.Value) != "litval" {
		return model.NewSymbol("nil"), fmt.Errorf("expected 'litval', got %v", verbTok.Value)
	}

	var args []model.Value
	for p.current.Type != TokenRParen && p.current.Type != TokenEOF {
		if p.current.Type == TokenAttribute {
			args = append(args, model.NewSymbol(model.NormalizeAttribute(p.current.Value)))
			if err := p.advance(); err != nil {
				return model.NewSymbol("nil"), err
			}
		} else if p.current.Type == TokenSymbol || p.current.Type == TokenVariable || p.current.Type == TokenString {
			args = append(args, TokenToValue(p.current))
			if err := p.advance(); err != nil {
				return model.NewSymbol("nil"), err
			}
		} else {
			return model.NewSymbol("nil"), fmt.Errorf("unexpected token in litval at line %d: %s", p.current.Line, p.current.Value)
		}
	}

	if _, err := p.expect(TokenRParen); err != nil {
		return model.NewSymbol("nil"), fmt.Errorf("expected ')' closing litval at line %d: %w", verbTok.Line, err)
	}

	if len(args) == 0 {
		return model.NewSymbol("nil"), fmt.Errorf("litval requires at least 1 argument (attribute name)")
	}
	if len(args) == 1 {
		return model.NewLitval("", args[0]), nil
	}
	class := args[0].String()
	if args[0].Type() == model.TypeSymbol || args[0].Type() == model.TypeString {
		class = args[0].Raw().(string)
	}
	return model.NewLitval(class, args[1]), nil
}

func (p *Parser) parseGenatom() (model.Value, error) {
	if _, err := p.expect(TokenLParen); err != nil {
		return model.NewSymbol("nil"), err
	}
	verbTok, err := p.expect(TokenSymbol)
	if err != nil || strings.ToLower(verbTok.Value) != "genatom" {
		return model.NewSymbol("nil"), fmt.Errorf("expected 'genatom', got %v", verbTok.Value)
	}
	if _, err := p.expect(TokenRParen); err != nil {
		return model.NewSymbol("nil"), fmt.Errorf("expected ')' closing genatom at line %d: %w", verbTok.Line, err)
	}
	return model.NewGenatom(), nil
}

func (p *Parser) parseAccept(isLine bool) (model.Value, error) {
	if _, err := p.expect(TokenLParen); err != nil {
		return model.NewSymbol("nil"), err
	}
	verbTok, err := p.expect(TokenSymbol)
	if err != nil {
		return model.NewSymbol("nil"), err
	}
	logicalFile := ""
	if p.current.Type != TokenRParen && p.current.Type != TokenEOF {
		logicalFile = p.current.Value
		if err := p.advance(); err != nil {
			return model.NewSymbol("nil"), err
		}
	}
	if _, err := p.expect(TokenRParen); err != nil {
		return model.NewSymbol("nil"), fmt.Errorf("expected ')' closing %s at line %d: %w", verbTok.Value, verbTok.Line, err)
	}
	return model.NewAccept(logicalFile, isLine), nil
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
		class, attrs, err := p.parseMakeBody(classTok)
		if err != nil {
			return nil, err
		}
		return model.MakeAction{Class: class, Attributes: attrs}, nil

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
			attrName := model.NormalizeAttribute(p.current.Value)
			if err := p.advance(); err != nil {
				return nil, err
			}
			var vals []model.Value
			for p.current.Type != TokenAttribute && p.current.Type != TokenRParen && p.current.Type != TokenEOF {
				if p.isRHSFunction() {
					fnVal, err := p.parseRHSFunction()
					if err != nil {
						return nil, err
					}
					vals = append(vals, fnVal)
				} else {
					vals = append(vals, TokenToValue(p.current))
					if err := p.advance(); err != nil {
						return nil, err
					}
				}
			}
			isVec := p.IsVectorAttribute(attrName)
			if len(vals) > 1 || isVec {
				attrs[attrName] = model.NewVector(vals)
			} else if len(vals) == 1 {
				attrs[attrName] = vals[0]
			} else {
				attrs[attrName] = model.NewSymbol("nil")
			}
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
		var wildcard bool
		if (p.current.Type == TokenOperator || p.current.Type == TokenSymbol) && p.current.Value == "*" {
			wildcard = true
			p.advance()
		} else if p.current.Type == TokenVariable {
			targetVar = p.current.Value
			p.advance()
		} else if p.current.Type == TokenNumber {
			idx, _ := strconv.Atoi(p.current.Value)
			targetIdx = idx
			p.advance()
		} else {
			return nil, fmt.Errorf("expected target variable, index, or '*' in remove action, got %s", p.current.Value)
		}
		if _, err := p.expect(TokenRParen); err != nil {
			return nil, err
		}
		return model.RemoveAction{
			TargetElementVar: targetVar,
			TargetIndex:      targetIdx,
			Wildcard:         wildcard,
		}, nil

	case "write":
		var args []model.WriteArg
		for p.current.Type != TokenRParen && p.current.Type != TokenEOF {
			if p.isRHSFunction() {
				fnVal, err := p.parseRHSFunction()
				if err != nil {
					return nil, err
				}
				args = append(args, model.WriteValue(fnVal))
				continue
			}
			if p.current.Type == TokenLParen {
				if p.peek.Type == TokenSymbol {
					subVerb := strings.ToLower(p.peek.Value)
					if subVerb == "crlf" {
						p.advance() // past '('
						p.advance() // past 'crlf'
						if _, err := p.expect(TokenRParen); err != nil {
							return nil, fmt.Errorf("expected ')' closing (crlf): %w", err)
						}
						args = append(args, model.WriteCRLF())
						continue
					}
					if subVerb == "tabto" {
						p.advance() // past '('
						p.advance() // past 'tabto'
						if p.current.Type == TokenRParen || p.current.Type == TokenEOF {
							return nil, fmt.Errorf("expected column argument in (tabto <col>) at line %d", p.current.Line)
						}
						colVal := TokenToValue(p.current)
						p.advance() // past colVal
						if _, err := p.expect(TokenRParen); err != nil {
							return nil, fmt.Errorf("expected ')' closing (tabto <col>): %w", err)
						}
						args = append(args, model.WriteTabTo(colVal))
						continue
					}
				}
				args = append(args, model.WriteValue(model.NewSymbol(p.current.Value)))
				p.advance()
			} else if p.current.Type == TokenSymbol && strings.ToLower(p.current.Value) == "crlf" {
				args = append(args, model.WriteCRLF())
				p.advance()
			} else {
				args = append(args, model.WriteValue(TokenToValue(p.current)))
				p.advance()
			}
		}
		if _, err := p.expect(TokenRParen); err != nil {
			return nil, err
		}
		return model.WriteAction{Args: args}, nil

	case "bind":
		if p.current.Type != TokenVariable && p.current.Type != TokenSymbol {
			return nil, fmt.Errorf("expected variable name in bind action at line %d, got %s", p.current.Line, p.current.Value)
		}
		varTok := p.current
		p.advance()

		var val model.Value
		if p.isRHSFunction() {
			compVal, err := p.parseRHSFunction()
			if err != nil {
				return nil, err
			}
			val = compVal
		} else {
			var vals []model.Value
			for p.current.Type != TokenRParen && p.current.Type != TokenEOF {
				if p.isRHSFunction() {
					compVal, err := p.parseRHSFunction()
					if err != nil {
						return nil, err
					}
					vals = append(vals, compVal)
				} else {
					vals = append(vals, TokenToValue(p.current))
					p.advance()
				}
			}
			if len(vals) == 1 {
				val = vals[0]
			} else if len(vals) > 1 {
				val = model.NewVector(vals)
			} else {
				val = model.NewSymbol("nil")
			}
		}
		if _, err := p.expect(TokenRParen); err != nil {
			return nil, err
		}
		return model.BindAction{
			Variable: varTok.Value,
			Value:    val,
		}, nil

	case "cbind":
		if p.current.Type != TokenVariable && p.current.Type != TokenSymbol {
			return nil, fmt.Errorf("expected element variable in cbind action at line %d, got %s", p.current.Line, p.current.Value)
		}
		varTok := p.current
		if err := p.advance(); err != nil {
			return nil, err
		}
		if _, err := p.expect(TokenRParen); err != nil {
			return nil, fmt.Errorf("expected ')' after element variable in cbind action at line %d: %w", varTok.Line, err)
		}
		return model.CBindAction{
			Variable: varTok.Value,
		}, nil

	case "openfile":
		act, err := p.parseOpenFileBody(verbTok.Line)
		if err != nil {
			return nil, err
		}
		return *act, nil

	case "closefile":
		act, err := p.parseCloseFileBody(verbTok.Line)
		if err != nil {
			return nil, err
		}
		return *act, nil

	case "default":
		act, err := p.parseDefaultBody(verbTok.Line)
		if err != nil {
			return nil, err
		}
		return *act, nil

	case "halt":
		if _, err := p.expect(TokenRParen); err != nil {
			return nil, err
		}
		return model.HaltAction{}, nil

	case "watch":
		level, err := p.parseWatchBody()
		if err != nil {
			return nil, err
		}
		return model.WatchAction{Level: level}, nil

	case "build":
		rule, err := p.ParseRule()
		if err != nil {
			return nil, fmt.Errorf("error parsing rule inside build action at line %d: %w", verbTok.Line, err)
		}
		if _, err := p.expect(TokenRParen); err != nil {
			return nil, fmt.Errorf("expected ')' closing build action at line %d: %w", verbTok.Line, err)
		}
		return model.BuildAction{Rule: rule}, nil

	default:
		// Check for bare make action: (ClassName ^attr val ...) or (ClassName attr val ...)
		if p.current.Type == TokenAttribute || p.isAttributeToken(p.current, p.getSchema(verbTok.Value)) || p.getSchema(verbTok.Value) != nil {
			class, attrs, err := p.parseMakeBody(verbTok)
			if err != nil {
				return nil, err
			}
			return model.MakeAction{Class: class, Attributes: attrs}, nil
		}

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

// StatementType represents the kind of top-level statement in an OPS5 source.
type StatementType int

const (
	StmtRule StatementType = iota
	StmtMake
	StmtLiteralize
	StmtVectorAttribute
	StmtOpenFile
	StmtCloseFile
	StmtDefault
	StmtExcise
	StmtPM
	StmtRemove
	StmtWatch
	StmtPPWM
	StmtStrategy
	StmtSubstr
	StmtMatches
	StmtPBreak
	StmtUnpbreak
	StmtDOT
)

func (st StatementType) String() string {
	switch st {
	case StmtRule:
		return "rule"
	case StmtMake:
		return "make"
	case StmtLiteralize:
		return "literalize"
	case StmtVectorAttribute:
		return "vector-attribute"
	case StmtOpenFile:
		return "openfile"
	case StmtCloseFile:
		return "closefile"
	case StmtDefault:
		return "default"
	case StmtExcise:
		return "excise"
	case StmtPM:
		return "pm"
	case StmtRemove:
		return "remove"
	case StmtWatch:
		return "watch"
	case StmtPPWM:
		return "ppwm"
	case StmtStrategy:
		return "strategy"
	case StmtSubstr:
		return "substr"
	case StmtMatches:
		return "matches"
	case StmtPBreak:
		return "pbreak"
	case StmtUnpbreak:
		return "unpbreak"
	case StmtDOT:
		return "dot"
	default:
		return "unknown"
	}
}

// Statement represents a parsed top-level OPS5 statement.
type Statement struct {
	Type            StatementType
	Rule            *model.Rule
	MakeClass       string
	MakeAttributes  map[string]model.Value
	LiteralizeClass string
	LiteralizeAttrs []string
	VectorAttrs     []string
	Schema          *model.ClassSchema
	OpenFile        *model.OpenFileAction
	CloseFile       *model.CloseFileAction
	Default         *model.DefaultAction
	ExciseRules     []string
	PMRules         []string
	RemoveWildcard  bool
	RemoveTimetags  []int64
	WatchLevel      *int
	PPWMPattern     *model.ConditionElement
	Strategy        string
	Substr          *model.SubstrExpr
	MatchesRules    []string
	PBreakRules     []string
	UnpbreakRules   []string
	DOTFile         string
}

func (p *Parser) parseMakeBody(classTok Token) (string, map[string]model.Value, error) {
	schema := p.getSchema(classTok.Value)
	attrs := make(map[string]model.Value)
	var orderedAttrs []string
	posIndex := 0

	for p.current.Type != TokenRParen && p.current.Type != TokenEOF {
		if p.isAttributeToken(p.current, schema) {
			attrName := model.NormalizeAttribute(p.current.Value)
			orderedAttrs = append(orderedAttrs, attrName)
			if schema != nil {
				schema.AddAttribute(attrName)
			}
			if err := p.advance(); err != nil {
				return "", nil, err
			}
			var vals []model.Value
			for !p.isAttributeToken(p.current, schema) && p.current.Type != TokenRParen && p.current.Type != TokenEOF {
				if p.isRHSFunction() {
					fnVal, err := p.parseRHSFunction()
					if err != nil {
						return "", nil, err
					}
					vals = append(vals, fnVal)
				} else {
					vals = append(vals, TokenToValue(p.current))
					if err := p.advance(); err != nil {
						return "", nil, err
					}
				}
			}
			isVec := p.IsVectorAttribute(attrName) || (schema != nil && schema.IsVectorAttribute(attrName))
			if len(vals) > 1 || isVec {
				attrs[attrName] = model.NewVector(vals)
			} else if len(vals) == 1 {
				attrs[attrName] = vals[0]
			} else {
				attrs[attrName] = model.NewSymbol("nil")
			}
		} else {
			var val model.Value
			if p.isRHSFunction() {
				fnVal, err := p.parseRHSFunction()
				if err != nil {
					return "", nil, err
				}
				val = fnVal
			} else {
				val = TokenToValue(p.current)
				if err := p.advance(); err != nil {
					return "", nil, err
				}
			}
			attrName := fmt.Sprintf("attr%d", posIndex+1)
			if schema != nil && posIndex < len(schema.Attributes) {
				attrName = schema.Attributes[posIndex]
			}
			orderedAttrs = append(orderedAttrs, attrName)
			isVec := p.IsVectorAttribute(attrName) || (schema != nil && schema.IsVectorAttribute(attrName))
			if isVec {
				if existing, ok := attrs[attrName]; ok && existing.IsVector() {
					attrs[attrName] = model.NewVector(append(existing.VectorElements(), val))
				} else {
					attrs[attrName] = model.NewVector([]model.Value{val})
				}
				if schema == nil || posIndex != len(schema.Attributes)-1 {
					posIndex++
				}
			} else {
				posIndex++
				attrs[attrName] = val
			}
		}
	}

	if _, err := p.expect(TokenRParen); err != nil {
		return "", nil, err
	}

	if schema == nil && len(orderedAttrs) > 0 {
		schema = model.NewClassSchema(classTok.Value, orderedAttrs)
		p.RegisterSchema(schema)
	} else if schema != nil && len(orderedAttrs) > 0 {
		for _, a := range orderedAttrs {
			schema.AddAttribute(a)
		}
	}

	return classTok.Value, attrs, nil
}

// ParseMake parses a standalone (make class [^attr val ...] [val1 val2 ...]) statement or bare (class ...).
func (p *Parser) ParseMake() (string, map[string]model.Value, error) {
	if _, err := p.expect(TokenLParen); err != nil {
		return "", nil, err
	}
	tok, err := p.expect(TokenSymbol)
	if err != nil {
		return "", nil, fmt.Errorf("expected class or 'make': %w", err)
	}

	var classTok Token
	if strings.ToLower(tok.Value) == "make" {
		cTok, err := p.expect(TokenSymbol)
		if err != nil {
			return "", nil, fmt.Errorf("expected class name after make: %w", err)
		}
		classTok = cTok
	} else {
		classTok = tok
	}

	return p.parseMakeBody(classTok)
}

// ParseLiteralize parses a (literalize class attr1 attr2 ...) directive.
func (p *Parser) ParseLiteralize() (string, []string, error) {
	if _, err := p.expect(TokenLParen); err != nil {
		return "", nil, err
	}
	verbTok, err := p.expect(TokenSymbol)
	if err != nil || strings.ToLower(verbTok.Value) != "literalize" {
		return "", nil, fmt.Errorf("expected 'literalize', got %v", verbTok.Value)
	}

	classTok, err := p.expect(TokenSymbol)
	if err != nil {
		return "", nil, fmt.Errorf("expected class name in literalize directive: %v", err)
	}

	var attrs []string
	for p.current.Type != TokenRParen && p.current.Type != TokenEOF {
		if p.current.Type == TokenSymbol || p.current.Type == TokenAttribute {
			attrs = append(attrs, model.NormalizeAttribute(p.current.Value))
			if err := p.advance(); err != nil {
				return "", nil, err
			}
		} else {
			return "", nil, fmt.Errorf("expected attribute name at line %d, got %s", p.current.Line, p.current.Value)
		}
	}

	if _, err := p.expect(TokenRParen); err != nil {
		return "", nil, err
	}

	schema := model.NewClassSchema(classTok.Value, attrs)
	p.RegisterSchema(schema)

	return classTok.Value, attrs, nil
}

// ParseVectorAttribute parses a (vector-attribute attr1 attr2 ...) directive.
func (p *Parser) ParseVectorAttribute() ([]string, error) {
	if _, err := p.expect(TokenLParen); err != nil {
		return nil, err
	}
	verbTok, err := p.expect(TokenSymbol)
	if err != nil || strings.ToLower(verbTok.Value) != "vector-attribute" {
		return nil, fmt.Errorf("expected 'vector-attribute', got %v", verbTok.Value)
	}

	var attrs []string
	for p.current.Type != TokenRParen && p.current.Type != TokenEOF {
		if p.current.Type == TokenSymbol || p.current.Type == TokenAttribute {
			norm := model.NormalizeAttribute(p.current.Value)
			attrs = append(attrs, norm)
			p.RegisterVectorAttribute(norm)
			if err := p.advance(); err != nil {
				return nil, err
			}
		} else {
			return nil, fmt.Errorf("expected attribute name at line %d, got %s", p.current.Line, p.current.Value)
		}
	}

	if _, err := p.expect(TokenRParen); err != nil {
		return nil, err
	}

	return attrs, nil
}

// ParseOpenFile parses a standalone (openfile logical-name filespec mode) statement.
func (p *Parser) ParseOpenFile() (*model.OpenFileAction, error) {
	if _, err := p.expect(TokenLParen); err != nil {
		return nil, err
	}
	verbTok, err := p.expect(TokenSymbol)
	if err != nil || strings.ToLower(verbTok.Value) != "openfile" {
		return nil, fmt.Errorf("expected 'openfile', got %v", verbTok.Value)
	}
	return p.parseOpenFileBody(verbTok.Line)
}

func (p *Parser) parseOpenFileBody(line int) (*model.OpenFileAction, error) {
	logTok, err := p.expect(TokenSymbol)
	if err != nil {
		return nil, fmt.Errorf("expected logical name in openfile at line %d: %w", line, err)
	}
	var filespec model.Value
	if p.current.Type == TokenSymbol || p.current.Type == TokenString || p.current.Type == TokenVariable {
		filespec = TokenToValue(p.current)
		if err := p.advance(); err != nil {
			return nil, err
		}
	} else {
		return nil, fmt.Errorf("expected filespec in openfile at line %d, got %v", p.current.Line, p.current.Value)
	}
	modeTok, err := p.expect(TokenSymbol)
	if err != nil {
		return nil, fmt.Errorf("expected mode (in, out, append) in openfile at line %d: %w", p.current.Line, err)
	}
	if _, err := p.expect(TokenRParen); err != nil {
		return nil, fmt.Errorf("expected ')' closing openfile at line %d: %w", modeTok.Line, err)
	}
	return &model.OpenFileAction{
		LogicalName: logTok.Value,
		Filespec:    filespec,
		Mode:        modeTok.Value,
	}, nil
}

// ParseCloseFile parses a standalone (closefile logical-name) statement.
func (p *Parser) ParseCloseFile() (*model.CloseFileAction, error) {
	if _, err := p.expect(TokenLParen); err != nil {
		return nil, err
	}
	verbTok, err := p.expect(TokenSymbol)
	if err != nil || strings.ToLower(verbTok.Value) != "closefile" {
		return nil, fmt.Errorf("expected 'closefile', got %v", verbTok.Value)
	}
	return p.parseCloseFileBody(verbTok.Line)
}

func (p *Parser) parseCloseFileBody(line int) (*model.CloseFileAction, error) {
	logTok, err := p.expect(TokenSymbol)
	if err != nil {
		return nil, fmt.Errorf("expected logical name in closefile at line %d: %w", line, err)
	}
	if _, err := p.expect(TokenRParen); err != nil {
		return nil, fmt.Errorf("expected ')' closing closefile at line %d: %w", logTok.Line, err)
	}
	return &model.CloseFileAction{
		LogicalName: logTok.Value,
	}, nil
}

// ParseDefault parses a standalone (default logical-name subsystem) statement.
func (p *Parser) ParseDefault() (*model.DefaultAction, error) {
	if _, err := p.expect(TokenLParen); err != nil {
		return nil, err
	}
	verbTok, err := p.expect(TokenSymbol)
	if err != nil || strings.ToLower(verbTok.Value) != "default" {
		return nil, fmt.Errorf("expected 'default', got %v", verbTok.Value)
	}
	return p.parseDefaultBody(verbTok.Line)
}

func (p *Parser) parseDefaultBody(line int) (*model.DefaultAction, error) {
	logTok, err := p.expect(TokenSymbol)
	if err != nil {
		return nil, fmt.Errorf("expected logical name in default at line %d: %w", line, err)
	}
	subTok, err := p.expect(TokenSymbol)
	if err != nil {
		return nil, fmt.Errorf("expected subsystem (accept, write, trace) in default at line %d: %w", p.current.Line, err)
	}
	if _, err := p.expect(TokenRParen); err != nil {
		return nil, fmt.Errorf("expected ')' closing default at line %d: %w", subTok.Line, err)
	}
	return &model.DefaultAction{
		LogicalName: logTok.Value,
		Subsystem:   subTok.Value,
	}, nil
}

// ParseExcise parses a standalone (excise rule-name-1 rule-name-2 ...) statement.
func (p *Parser) ParseExcise() ([]string, error) {
	if _, err := p.expect(TokenLParen); err != nil {
		return nil, err
	}
	verbTok, err := p.expect(TokenSymbol)
	if err != nil || strings.ToLower(verbTok.Value) != "excise" {
		return nil, fmt.Errorf("expected 'excise', got %v", verbTok.Value)
	}
	var names []string
	for p.current.Type != TokenRParen && p.current.Type != TokenEOF {
		tok, err := p.expect(TokenSymbol)
		if err != nil {
			return nil, fmt.Errorf("expected rule name in excise statement: %w", err)
		}
		names = append(names, tok.Value)
	}
	if len(names) == 0 {
		return nil, fmt.Errorf("expected at least one rule name in excise statement")
	}
	if _, err := p.expect(TokenRParen); err != nil {
		return nil, fmt.Errorf("expected ')' closing excise statement: %w", err)
	}
	return names, nil
}

// ParsePM parses a standalone (pm [rule-name-1 ... | *]) statement.
func (p *Parser) ParsePM() ([]string, error) {
	if _, err := p.expect(TokenLParen); err != nil {
		return nil, err
	}
	verbTok, err := p.expect(TokenSymbol)
	if err != nil || strings.ToLower(verbTok.Value) != "pm" {
		return nil, fmt.Errorf("expected 'pm', got %v", verbTok.Value)
	}
	var names []string
	for p.current.Type != TokenRParen && p.current.Type != TokenEOF {
		if p.current.Type == TokenOperator && p.current.Value == "*" {
			names = append(names, "*")
			if err := p.advance(); err != nil {
				return nil, err
			}
		} else if p.current.Type == TokenSymbol {
			names = append(names, p.current.Value)
			if err := p.advance(); err != nil {
				return nil, err
			}
		} else {
			return nil, fmt.Errorf("expected rule name or '*' in pm statement, got %s (%q)", p.current.Type, p.current.Value)
		}
	}
	if _, err := p.expect(TokenRParen); err != nil {
		return nil, fmt.Errorf("expected ')' closing pm statement: %w", err)
	}
	return names, nil
}

// ParseTopLevelRemove parses a top-level (remove [timetag... | *]) statement.
func (p *Parser) ParseTopLevelRemove() (*Statement, error) {
	if _, err := p.expect(TokenLParen); err != nil {
		return nil, err
	}
	verbTok, err := p.expect(TokenSymbol)
	if err != nil || strings.ToLower(verbTok.Value) != "remove" {
		return nil, fmt.Errorf("expected 'remove', got %v", verbTok.Value)
	}
	var timetags []int64
	var isWildcard bool
	for p.current.Type != TokenRParen && p.current.Type != TokenEOF {
		if (p.current.Type == TokenOperator || p.current.Type == TokenSymbol) && p.current.Value == "*" {
			isWildcard = true
			if err := p.advance(); err != nil {
				return nil, err
			}
		} else if p.current.Type == TokenNumber {
			val, err := strconv.ParseInt(p.current.Value, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("invalid timetag in remove statement: %w", err)
			}
			timetags = append(timetags, val)
			if err := p.advance(); err != nil {
				return nil, err
			}
		} else {
			return nil, fmt.Errorf("expected timetag or '*' in remove statement, got %s (%q)", p.current.Type, p.current.Value)
		}
	}
	if _, err := p.expect(TokenRParen); err != nil {
		return nil, fmt.Errorf("expected ')' closing remove statement: %w", err)
	}
	if !isWildcard && len(timetags) == 0 {
		return nil, fmt.Errorf("expected at least one timetag or '*' in remove statement")
	}
	return &Statement{
		Type:           StmtRemove,
		RemoveWildcard: isWildcard,
		RemoveTimetags: timetags,
	}, nil
}

// ParseWatch parses a top-level (watch [0|1|2]) statement.
func (p *Parser) ParseWatch() (*int, error) {
	if _, err := p.expect(TokenLParen); err != nil {
		return nil, err
	}
	verbTok, err := p.expect(TokenSymbol)
	if err != nil || strings.ToLower(verbTok.Value) != "watch" {
		return nil, fmt.Errorf("expected 'watch', got %v", verbTok.Value)
	}
	return p.parseWatchBody()
}

func (p *Parser) parseWatchBody() (*int, error) {
	if p.current.Type == TokenRParen {
		if _, err := p.expect(TokenRParen); err != nil {
			return nil, err
		}
		return nil, nil
	}

	if p.current.Type != TokenNumber {
		return nil, fmt.Errorf("expected number or ')' in watch statement, got %s (%q) at line %d, col %d",
			p.current.Type, p.current.Value, p.current.Line, p.current.Col)
	}

	lvl, err := strconv.Atoi(p.current.Value)
	if err != nil {
		return nil, fmt.Errorf("invalid watch level %q at line %d: %w", p.current.Value, p.current.Line, err)
	}
	if lvl < 0 || lvl > 2 {
		return nil, fmt.Errorf("invalid watch level %d (expected 0, 1, or 2) at line %d", lvl, p.current.Line)
	}

	if err := p.advance(); err != nil {
		return nil, err
	}

	if _, err := p.expect(TokenRParen); err != nil {
		return nil, fmt.Errorf("expected ')' closing watch statement: %w", err)
	}

	return &lvl, nil
}

func checkPPWMForbiddenToken(tok Token) error {
	if tok.Type == TokenVariable {
		return fmt.Errorf("ppwm pattern cannot contain variables (found '<%s>')", tok.Value)
	}
	if tok.Type == TokenLBrace || tok.Type == TokenRBrace || tok.Value == "{" || tok.Value == "}" {
		return fmt.Errorf("ppwm pattern cannot contain curly braces")
	}
	if tok.Value == "//" {
		return fmt.Errorf("ppwm pattern cannot contain the quote operator '//'")
	}
	if tok.Type == TokenNegation {
		return fmt.Errorf("ppwm pattern cannot contain negation '-'")
	}
	if tok.Type == TokenOperator && tok.Value != "*" {
		return fmt.Errorf("ppwm pattern cannot contain predicates (found '%s')", tok.Value)
	}
	if strings.Contains(tok.Value, "<") || strings.Contains(tok.Value, ">") {
		return fmt.Errorf("ppwm pattern cannot contain angle brackets (found '%s')", tok.Value)
	}
	return nil
}

// ParsePPWM parses a top-level (ppwm [pattern]) statement.
// The pattern takes the form of an LHS condition element (e.g. City ^state Pennsylvania).
// In accordance with classic OPS5 specifications, the pattern cannot contain variables,
// predicates (i.e. <, >, !=, ...), the quote operator //, angle brackets or curly braces.
func (p *Parser) ParsePPWM() (*model.ConditionElement, error) {
	if _, err := p.expect(TokenLParen); err != nil {
		return nil, err
	}
	verbTok, err := p.expect(TokenSymbol)
	if err != nil || !strings.EqualFold(verbTok.Value, "ppwm") {
		return nil, fmt.Errorf("expected 'ppwm', got %v", verbTok.Value)
	}

	// (ppwm) without arguments
	if p.current.Type == TokenRParen {
		if _, err := p.expect(TokenRParen); err != nil {
			return nil, err
		}
		return model.NewPositiveCE("*"), nil
	}

	// (ppwm *)
	if (p.current.Type == TokenOperator || p.current.Type == TokenSymbol) && p.current.Value == "*" {
		if err := p.advance(); err != nil {
			return nil, err
		}
		if p.current.Type == TokenRParen {
			if _, err := p.expect(TokenRParen); err != nil {
				return nil, err
			}
			return model.NewPositiveCE("*"), nil
		}
	}

	// Check if user enclosed the pattern in inner parentheses: (ppwm (City ^state Pennsylvania))
	hasInnerParens := false
	if p.current.Type == TokenLParen {
		hasInnerParens = true
		if err := p.advance(); err != nil {
			return nil, err
		}
	}

	// Check for forbidden constructs before class name
	if err := checkPPWMForbiddenToken(p.current); err != nil {
		return nil, err
	}

	var className string
	if (p.current.Type == TokenOperator || p.current.Type == TokenSymbol) && p.current.Value == "*" {
		className = "*"
		if err := p.advance(); err != nil {
			return nil, err
		}
	} else if p.current.Type == TokenSymbol || p.current.Type == TokenString {
		className = p.current.Value
		if err := p.advance(); err != nil {
			return nil, err
		}
	} else {
		return nil, fmt.Errorf("expected class name in ppwm pattern, got %s (%q)", p.current.Type, p.current.Value)
	}

	ce := model.NewPositiveCE(className)
	schema := p.getSchema(className)
	posIndex := 0

	// Loop over attributes and values until RParen or EOF
	for p.current.Type != TokenRParen && p.current.Type != TokenEOF {
		if err := checkPPWMForbiddenToken(p.current); err != nil {
			return nil, err
		}

		if p.isAttributeToken(p.current, schema) {
			attrName := model.NormalizeAttribute(p.current.Value)
			if err := p.advance(); err != nil {
				return nil, err
			}

			var vals []model.Value
			for !p.isAttributeToken(p.current, schema) && p.current.Type != TokenRParen && p.current.Type != TokenEOF {
				if err := checkPPWMForbiddenToken(p.current); err != nil {
					return nil, err
				}
				vals = append(vals, TokenToValue(p.current))
				if err := p.advance(); err != nil {
					return nil, err
				}
			}

			isVec := p.IsVectorAttribute(attrName) || (schema != nil && schema.IsVectorAttribute(attrName))
			if len(vals) == 0 {
				ce.AddEqualTest(attrName, model.NewSymbol("nil"))
			} else if len(vals) == 1 && !isVec {
				ce.AddEqualTest(attrName, vals[0])
			} else {
				ce.AddEqualTest(attrName, model.NewVector(vals))
			}
		} else {
			// Positional attribute value
			attrName := fmt.Sprintf("attr%d", posIndex+1)
			if schema != nil && posIndex < len(schema.Attributes) {
				attrName = schema.Attributes[posIndex]
			}
			isVec := p.IsVectorAttribute(attrName) || (schema != nil && schema.IsVectorAttribute(attrName))

			val := TokenToValue(p.current)
			if err := p.advance(); err != nil {
				return nil, err
			}

			if isVec {
				found := false
				for i := range ce.Tests {
					if ce.Tests[i].Attribute == attrName && len(ce.Tests[i].Constraints) > 0 {
						curVal := ce.Tests[i].Constraints[0].Value
						if curVal.IsVector() {
							ce.Tests[i].Constraints[0].Value = model.NewVector(append(curVal.VectorElements(), val))
						} else {
							ce.Tests[i].Constraints[0].Value = model.NewVector([]model.Value{curVal, val})
						}
						found = true
						break
					}
				}
				if !found {
					ce.AddEqualTest(attrName, val)
				}
				if schema == nil || posIndex != len(schema.Attributes)-1 {
					posIndex++
				}
			} else {
				posIndex++
				ce.AddEqualTest(attrName, val)
			}
		}
	}

	if hasInnerParens {
		if _, err := p.expect(TokenRParen); err != nil {
			return nil, fmt.Errorf("expected ')' closing condition element in ppwm: %w", err)
		}
	}

	if _, err := p.expect(TokenRParen); err != nil {
		return nil, fmt.Errorf("expected ')' closing ppwm statement: %w", err)
	}

	return ce, nil
}

// ParseStrategy parses a top-level (strategy [LEX|MEA]) statement.
func (p *Parser) ParseStrategy() (string, error) {
	if _, err := p.expect(TokenLParen); err != nil {
		return "", err
	}
	verbTok, err := p.expect(TokenSymbol)
	if err != nil || strings.ToLower(verbTok.Value) != "strategy" {
		return "", fmt.Errorf("expected 'strategy', got %v", verbTok.Value)
	}
	if p.current.Type == TokenRParen {
		if _, err := p.expect(TokenRParen); err != nil {
			return "", err
		}
		return "", nil
	}
	valTok, err := p.expect(TokenSymbol)
	if err != nil {
		return "", err
	}
	val := strings.ToUpper(valTok.Value)
	if val != "LEX" && val != "MEA" {
		return "", fmt.Errorf("unknown conflict resolution strategy %q (must be LEX or MEA)", valTok.Value)
	}
	if _, err := p.expect(TokenRParen); err != nil {
		return "", fmt.Errorf("expected ')' closing strategy statement: %w", err)
	}
	return val, nil
}

// ParseMatches parses a standalone (matches [rule-name-1 ... | *]) statement.
func (p *Parser) ParseMatches() ([]string, error) {
	if _, err := p.expect(TokenLParen); err != nil {
		return nil, err
	}
	verbTok, err := p.expect(TokenSymbol)
	if err != nil || strings.ToLower(verbTok.Value) != "matches" {
		return nil, fmt.Errorf("expected 'matches', got %v", verbTok.Value)
	}
	var names []string
	for p.current.Type != TokenRParen && p.current.Type != TokenEOF {
		if p.current.Type == TokenOperator && p.current.Value == "*" {
			names = append(names, "*")
			if err := p.advance(); err != nil {
				return nil, err
			}
		} else if p.current.Type == TokenSymbol {
			names = append(names, p.current.Value)
			if err := p.advance(); err != nil {
				return nil, err
			}
		} else {
			return nil, fmt.Errorf("expected rule name or '*' in matches statement, got %s (%q)", p.current.Type, p.current.Value)
		}
	}
	if _, err := p.expect(TokenRParen); err != nil {
		return nil, fmt.Errorf("expected ')' closing matches statement: %w", err)
	}
	return names, nil
}

// ParsePBreak parses a standalone (pbreak [rule-name-1 ...]) statement.
func (p *Parser) ParsePBreak() ([]string, error) {
	if _, err := p.expect(TokenLParen); err != nil {
		return nil, err
	}
	verbTok, err := p.expect(TokenSymbol)
	if err != nil || strings.ToLower(verbTok.Value) != "pbreak" {
		return nil, fmt.Errorf("expected 'pbreak', got %v", verbTok.Value)
	}
	var names []string
	for p.current.Type != TokenRParen && p.current.Type != TokenEOF {
		if p.current.Type == TokenSymbol {
			names = append(names, p.current.Value)
			if err := p.advance(); err != nil {
				return nil, err
			}
		} else {
			return nil, fmt.Errorf("expected rule name in pbreak statement, got %s (%q)", p.current.Type, p.current.Value)
		}
	}
	if _, err := p.expect(TokenRParen); err != nil {
		return nil, fmt.Errorf("expected ')' closing pbreak statement: %w", err)
	}
	return names, nil
}

// ParseUnpbreak parses a standalone (unpbreak [rule-name-1 ... | * | nil]) statement.
func (p *Parser) ParseUnpbreak() ([]string, error) {
	if _, err := p.expect(TokenLParen); err != nil {
		return nil, err
	}
	verbTok, err := p.expect(TokenSymbol)
	if err != nil || (strings.ToLower(verbTok.Value) != "unpbreak" && strings.ToLower(verbTok.Value) != "unbreak") {
		return nil, fmt.Errorf("expected 'unpbreak' or 'unbreak', got %v", verbTok.Value)
	}
	var names []string
	for p.current.Type != TokenRParen && p.current.Type != TokenEOF {
		if p.current.Type == TokenOperator && p.current.Value == "*" {
			names = append(names, "*")
			if err := p.advance(); err != nil {
				return nil, err
			}
		} else if p.current.Type == TokenSymbol {
			names = append(names, p.current.Value)
			if err := p.advance(); err != nil {
				return nil, err
			}
		} else {
			return nil, fmt.Errorf("expected rule name, '*', or 'nil' in unpbreak statement, got %s (%q)", p.current.Type, p.current.Value)
		}
	}
	if _, err := p.expect(TokenRParen); err != nil {
		return nil, fmt.Errorf("expected ')' closing unpbreak statement: %w", err)
	}
	return names, nil
}

// ParseDOT parses a standalone (dot [filepath]) statement.
func (p *Parser) ParseDOT() (string, error) {
	if _, err := p.expect(TokenLParen); err != nil {
		return "", err
	}
	verbTok, err := p.expect(TokenSymbol)
	if err != nil || strings.ToLower(verbTok.Value) != "dot" {
		return "", fmt.Errorf("expected 'dot', got %v", verbTok.Value)
	}
	var filePath string
	if p.current.Type == TokenSymbol || p.current.Type == TokenString {
		filePath = p.current.Value
		if err := p.advance(); err != nil {
			return "", err
		}
	}
	if _, err := p.expect(TokenRParen); err != nil {
		return "", fmt.Errorf("expected ')' closing dot statement: %w", err)
	}
	return filePath, nil
}

// NextStatement parses the next top-level statement (Rule, Make, Literalize, VectorAttribute, OpenFile, CloseFile, Default, Excise, PM, Remove, Watch, PPWM, or Strategy).
// Returns (nil, nil) when TokenEOF is reached.
func (p *Parser) NextStatement() (*Statement, error) {
	if p.current.Type == TokenEOF {
		return nil, nil
	}

	if p.current.Type != TokenLParen {
		return nil, fmt.Errorf("expected '(' starting statement at line %d, got %s (%q)",
			p.current.Line, p.current.Type, p.current.Value)
	}

	verb := strings.ToLower(p.peek.Value)
	switch verb {
	case "p":
		rule, err := p.ParseRule()
		if err != nil {
			return nil, err
		}
		return &Statement{Type: StmtRule, Rule: rule}, nil

	case "build":
		if err := p.advance(); err != nil { // '('
			return nil, err
		}
		if err := p.advance(); err != nil { // 'build'
			return nil, err
		}
		rule, err := p.ParseRule()
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(TokenRParen); err != nil {
			return nil, fmt.Errorf("expected ')' closing build statement: %w", err)
		}
		return &Statement{Type: StmtRule, Rule: rule}, nil

	case "make":
		class, attrs, err := p.ParseMake()
		if err != nil {
			return nil, err
		}
		return &Statement{Type: StmtMake, MakeClass: class, MakeAttributes: attrs, Schema: p.getSchema(class)}, nil

	case "literalize":
		class, attrs, err := p.ParseLiteralize()
		if err != nil {
			return nil, err
		}
		schema := p.getSchema(class)
		return &Statement{
			Type:            StmtLiteralize,
			LiteralizeClass: class,
			LiteralizeAttrs: attrs,
			Schema:          schema,
		}, nil

	case "vector-attribute":
		attrs, err := p.ParseVectorAttribute()
		if err != nil {
			return nil, err
		}
		return &Statement{
			Type:        StmtVectorAttribute,
			VectorAttrs: attrs,
		}, nil

	case "openfile":
		act, err := p.ParseOpenFile()
		if err != nil {
			return nil, err
		}
		return &Statement{
			Type:     StmtOpenFile,
			OpenFile: act,
		}, nil

	case "closefile":
		act, err := p.ParseCloseFile()
		if err != nil {
			return nil, err
		}
		return &Statement{
			Type:      StmtCloseFile,
			CloseFile: act,
		}, nil

	case "default":
		act, err := p.ParseDefault()
		if err != nil {
			return nil, err
		}
		return &Statement{
			Type:    StmtDefault,
			Default: act,
		}, nil

	case "excise":
		rules, err := p.ParseExcise()
		if err != nil {
			return nil, err
		}
		return &Statement{
			Type:        StmtExcise,
			ExciseRules: rules,
		}, nil

	case "pm":
		rules, err := p.ParsePM()
		if err != nil {
			return nil, err
		}
		return &Statement{
			Type:    StmtPM,
			PMRules: rules,
		}, nil

	case "remove":
		stmt, err := p.ParseTopLevelRemove()
		if err != nil {
			return nil, err
		}
		return stmt, nil

	case "watch":
		level, err := p.ParseWatch()
		if err != nil {
			return nil, err
		}
		return &Statement{
			Type:       StmtWatch,
			WatchLevel: level,
		}, nil

	case "ppwm":
		ce, err := p.ParsePPWM()
		if err != nil {
			return nil, err
		}
		return &Statement{
			Type:        StmtPPWM,
			PPWMPattern: ce,
		}, nil

	case "strategy":
		strat, err := p.ParseStrategy()
		if err != nil {
			return nil, err
		}
		return &Statement{
			Type:     StmtStrategy,
			Strategy: strat,
		}, nil

	case "substr":
		val, err := p.ParseSubstr()
		if err != nil {
			return nil, err
		}
		return &Statement{
			Type:   StmtSubstr,
			Substr: val.SubstrExpr(),
		}, nil

	case "matches":
		rules, err := p.ParseMatches()
		if err != nil {
			return nil, err
		}
		return &Statement{
			Type:         StmtMatches,
			MatchesRules: rules,
		}, nil

	case "pbreak":
		rules, err := p.ParsePBreak()
		if err != nil {
			return nil, err
		}
		return &Statement{
			Type:        StmtPBreak,
			PBreakRules: rules,
		}, nil

	case "unpbreak", "unbreak":
		rules, err := p.ParseUnpbreak()
		if err != nil {
			return nil, err
		}
		return &Statement{
			Type:          StmtUnpbreak,
			UnpbreakRules: rules,
		}, nil

	case "dot":
		filePath, err := p.ParseDOT()
		if err != nil {
			return nil, err
		}
		return &Statement{
			Type:    StmtDOT,
			DOTFile: filePath,
		}, nil

	default:
		if p.peek.Type == TokenSymbol {
			class, attrs, err := p.ParseMake()
			if err != nil {
				return nil, err
			}
			return &Statement{
				Type:           StmtMake,
				MakeClass:      class,
				MakeAttributes: attrs,
				Schema:         p.getSchema(class),
			}, nil
		}
		return nil, fmt.Errorf("unknown statement verb %q at line %d, col %d",
			p.peek.Value, p.peek.Line, p.peek.Col)
	}
}

// ParseRules parses all rules from an OPS5 source string, processing any literalize schemas encountered.
func ParseRules(source string) ([]*model.Rule, error) {
	p, err := NewParser(source)
	if err != nil {
		return nil, err
	}

	var rules []*model.Rule
	for {
		stmt, err := p.NextStatement()
		if err != nil {
			return nil, err
		}
		if stmt == nil {
			break
		}
		if stmt.Type == StmtRule {
			rules = append(rules, stmt.Rule)
		}
	}

	return rules, nil
}

// ParseProgram parses an entire OPS5 source string into rules, makes, and literalize statements.
func ParseProgram(source string) ([]*Statement, error) {
	p, err := NewParser(source)
	if err != nil {
		return nil, err
	}

	var stmts []*Statement
	for {
		stmt, err := p.NextStatement()
		if err != nil {
			return nil, err
		}
		if stmt == nil {
			break
		}
		stmts = append(stmts, stmt)
	}

	return stmts, nil
}

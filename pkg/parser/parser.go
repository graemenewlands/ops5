package parser

import (
	"fmt"
	"strconv"
	"strings"

	"ops5/pkg/model"
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

	schema := p.getSchema(classTok.Value)
	posIndex := 0

	// Parse attribute tests (both named ^attr and positional)
	for p.current.Type != TokenRParen && p.current.Type != TokenEOF {
		if p.current.Type == TokenAttribute {
			attrName := model.NormalizeAttribute(p.current.Value)
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
		} else {
			// Positional test in condition element
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

			attrName := fmt.Sprintf("attr%d", posIndex+1)
			if schema != nil && posIndex < len(schema.Attributes) {
				attrName = schema.Attributes[posIndex]
			}
			posIndex++

			ce.AddTest(attrName, op, val)
		}
	}

	if _, err := p.expect(TokenRParen); err != nil {
		return nil, err
	}

	return ce, nil
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
		if p.current.Type == TokenLParen && strings.ToLower(p.peek.Value) == "compute" {
			subComp, err := p.parseCompute()
			if err != nil {
				return model.NewInt(0), err
			}
			operands = append(operands, subComp)
		} else if p.current.Type == TokenNumber || p.current.Type == TokenVariable || p.current.Type == TokenSymbol {
			operands = append(operands, TokenToValue(p.current))
			if err := p.advance(); err != nil {
				return model.NewInt(0), err
			}
		} else {
			return model.NewInt(0), fmt.Errorf("expected number, variable, or nested compute in compute expression at line %d, got %v", p.current.Line, p.current.Value)
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
		schema := p.getSchema(classTok.Value)
		attrs := make(map[string]model.Value)
		posIndex := 0
		for p.current.Type != TokenRParen && p.current.Type != TokenEOF {
			if p.current.Type == TokenAttribute {
				attrName := model.NormalizeAttribute(p.current.Value)
				if err := p.advance(); err != nil {
					return nil, err
				}
				var vals []model.Value
				for p.current.Type != TokenAttribute && p.current.Type != TokenRParen && p.current.Type != TokenEOF {
					if p.current.Type == TokenLParen && strings.ToLower(p.peek.Value) == "compute" {
						compVal, err := p.parseCompute()
						if err != nil {
							return nil, err
						}
						vals = append(vals, compVal)
					} else {
						vals = append(vals, TokenToValue(p.current))
						if err := p.advance(); err != nil {
							return nil, err
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
				if p.current.Type == TokenLParen && strings.ToLower(p.peek.Value) == "compute" {
					compVal, err := p.parseCompute()
					if err != nil {
						return nil, err
					}
					val = compVal
				} else {
					val = TokenToValue(p.current)
					if err := p.advance(); err != nil {
						return nil, err
					}
				}
				attrName := fmt.Sprintf("attr%d", posIndex+1)
				if schema != nil && posIndex < len(schema.Attributes) {
					attrName = schema.Attributes[posIndex]
				}
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
			attrName := model.NormalizeAttribute(p.current.Value)
			if err := p.advance(); err != nil {
				return nil, err
			}
			var vals []model.Value
			for p.current.Type != TokenAttribute && p.current.Type != TokenRParen && p.current.Type != TokenEOF {
				if p.current.Type == TokenLParen && strings.ToLower(p.peek.Value) == "compute" {
					compVal, err := p.parseCompute()
					if err != nil {
						return nil, err
					}
					vals = append(vals, compVal)
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
		var args []model.WriteArg
		for p.current.Type != TokenRParen && p.current.Type != TokenEOF {
			if p.current.Type == TokenLParen {
				if p.peek.Type == TokenSymbol {
					subVerb := strings.ToLower(p.peek.Value)
					if subVerb == "compute" {
						compVal, err := p.parseCompute()
						if err != nil {
							return nil, err
						}
						args = append(args, model.WriteValue(compVal))
						continue
					}
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
		if p.current.Type == TokenLParen && strings.ToLower(p.peek.Value) == "compute" {
			compVal, err := p.parseCompute()
			if err != nil {
				return nil, err
			}
			val = compVal
		} else {
			var vals []model.Value
			for p.current.Type != TokenRParen && p.current.Type != TokenEOF {
				if p.current.Type == TokenLParen && strings.ToLower(p.peek.Value) == "compute" {
					compVal, err := p.parseCompute()
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

// StatementType represents the kind of top-level statement in an OPS5 source.
type StatementType int

const (
	StmtRule StatementType = iota
	StmtMake
	StmtLiteralize
	StmtVectorAttribute
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
}

// ParseMake parses a standalone (make class [^attr val ...] [val1 val2 ...]) statement.
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

	schema := p.getSchema(classTok.Value)
	attrs := make(map[string]model.Value)
	posIndex := 0

	for p.current.Type != TokenRParen && p.current.Type != TokenEOF {
		if p.current.Type == TokenAttribute {
			attrName := model.NormalizeAttribute(p.current.Value)
			if err := p.advance(); err != nil {
				return "", nil, err
			}
			var vals []model.Value
			for p.current.Type != TokenAttribute && p.current.Type != TokenRParen && p.current.Type != TokenEOF {
				vals = append(vals, TokenToValue(p.current))
				if err := p.advance(); err != nil {
					return "", nil, err
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
			val := TokenToValue(p.current)
			if err := p.advance(); err != nil {
				return "", nil, err
			}
			attrName := fmt.Sprintf("attr%d", posIndex+1)
			if schema != nil && posIndex < len(schema.Attributes) {
				attrName = schema.Attributes[posIndex]
			}
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

	return classTok.Value, attrs, nil
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

// NextStatement parses the next top-level statement (Rule, Make, Literalize, or VectorAttribute).
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

	case "make":
		class, attrs, err := p.ParseMake()
		if err != nil {
			return nil, err
		}
		return &Statement{Type: StmtMake, MakeClass: class, MakeAttributes: attrs}, nil

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

	default:
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

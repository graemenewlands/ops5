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
			// Positional test in condition element
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
	return sub == "compute" || sub == "accept" || sub == "acceptline" || sub == "genatom" || sub == "litval"
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
	default:
		return model.NewSymbol("nil"), fmt.Errorf("unknown RHS function: %s", sub)
	}
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

// NextStatement parses the next top-level statement (Rule, Make, Literalize, VectorAttribute, OpenFile, CloseFile, Default, or Excise).
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

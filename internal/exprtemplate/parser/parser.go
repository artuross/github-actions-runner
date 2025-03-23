package parser

import (
	"errors"
	"fmt"
	"io"
	"strconv"

	"github.com/artuross/github-actions-runner/internal/exprtemplate/ast"
	"github.com/artuross/github-actions-runner/internal/exprtemplate/lexer"
)

type OperatorPrecedence int

const (
	OperatorPrecedenceUnset       OperatorPrecedence = 0
	OperatorPrecedenceClose       OperatorPrecedence = 1  // ")" "]" ","
	OperatorPrecedenceOr          OperatorPrecedence = 5  // "||"
	OperatorPrecedenceAnd         OperatorPrecedence = 6  // "&&"
	OperatorPrecedenceEquality    OperatorPrecedence = 10 // "==" "!="
	OperatorPrecedenceGreaterLess OperatorPrecedence = 11 // ">" ">=" "<" "<="
	OperatorPrecedenceNegation    OperatorPrecedence = 16 // "!"
	OperatorPrecedenceStart       OperatorPrecedence = 19 // "(" "[" "."
	OperatorPrecedenceFunction    OperatorPrecedence = 20 // "(" (function call)
)

type OperatorType int

const (
	OperatorTypeUnset OperatorType = iota
	OperatorTypeBinary
	OperatorTypeUnary
)

type Operator struct {
	token  string
	prec   OperatorPrecedence
	opType OperatorType
}

var operators = map[string]Operator{
	"||": {"||", OperatorPrecedenceOr, OperatorTypeBinary},
	"&&": {"&&", OperatorPrecedenceAnd, OperatorTypeBinary},
	"==": {"==", OperatorPrecedenceEquality, OperatorTypeBinary},
	"!=": {"!=", OperatorPrecedenceEquality, OperatorTypeBinary},
	">":  {">", OperatorPrecedenceGreaterLess, OperatorTypeBinary},
	"<":  {"<", OperatorPrecedenceGreaterLess, OperatorTypeBinary},
	">=": {">=", OperatorPrecedenceGreaterLess, OperatorTypeBinary},
	"<=": {"<=", OperatorPrecedenceGreaterLess, OperatorTypeBinary},
	"!":  {"!", OperatorPrecedenceNegation, OperatorTypeUnary},
}

type Lexer interface {
	ReadToken() (*lexer.Token, error)
}

type Parser struct {
	lexer  Lexer
	tokens []*lexer.Token
	pos    int
}

func NewParser(lexer Lexer) *Parser {
	return &Parser{
		lexer: lexer,
	}
}

func (p *Parser) Parse() (ast.Expr, error) {
	return p.parseBinaryExpression(OperatorPrecedenceUnset)
}

func (p *Parser) expectPunctuation(value string) error {
	token, err := p.readToken()
	if err == io.EOF {
		return io.ErrUnexpectedEOF
	}
	if err != nil {
		return err
	}

	if token.Type != lexer.TokenTypePunctuation || token.Value != value {
		return fmt.Errorf("expected punctuation: '%s'", value)
	}

	return nil
}

func (p *Parser) parseBinaryExpression(minPrec OperatorPrecedence) (ast.Expr, error) {
	left, err := p.parsePrimaryExpression()
	if err != nil {
		return nil, err
	}

	for {
		token, err := p.readToken()
		if err == io.EOF {
			return left, nil
		}
		if err != nil {
			return nil, err
		}

		if token.Type == lexer.TokenTypePunctuation {
			if token.Value == ")" {
				p.unreadToken()

				return left, nil
			}

			if token.Value == "," {
				p.unreadToken()

				return left, nil
			}

			op, isOp := operators[token.Value]
			if !isOp {
				return nil, fmt.Errorf("unsupported operator: %s", token.Value)
			}

			if op.opType != OperatorTypeBinary {
				return nil, fmt.Errorf("expected binary operator")
			}

			if op.prec < minPrec {
				// we shouldn't consume this token here
				p.unreadToken()

				return left, nil
			}

			right, err := p.parseBinaryExpression(op.prec + 1)
			if err == io.EOF {
				return nil, io.ErrUnexpectedEOF
			}
			if err != nil {
				return nil, err
			}

			left = &ast.Binary{
				Left:     left,
				Operator: token.Value,
				Right:    right,
			}
		}
	}
}

func (p *Parser) parseCallArgumentList() ([]ast.Expr, error) {
	args := make([]ast.Expr, 0)
	for {
		// parse arg
		arg, err := p.parseExpression()
		if err != nil {
			return nil, err
		}

		// expect either , or )
		token, err := p.readToken()
		if err != nil {
			return nil, err
		}

		// no arg
		if token.Type == lexer.TokenTypePunctuation && token.Value == ")" {
			p.unreadToken()

			args = append(args, arg)
			break
		}

		// has next arg
		if token.Type == lexer.TokenTypePunctuation && token.Value == "," {
			args = append(args, arg)
			continue
		}

		return nil, errors.New("expected , or ) after an argument in a function call")
	}

	return args, nil
}

func (p *Parser) parseCallExpression(base ast.Expr) (ast.Expr, error) {
	// check if there's an arg
	token, err := p.peekToken()
	if err != nil {
		return nil, err
	}

	args := make([]ast.Expr, 0)
	if token.Type != lexer.TokenTypePunctuation && token.Value != ")" {
		argList, err := p.parseCallArgumentList()
		if err != nil {
			return nil, err
		}

		args = argList
	}

	// closing
	if err := p.expectPunctuation(")"); err != nil {
		return nil, errors.New("expected ')'")
	}

	// TODO: parse arguments
	expr := &ast.FunctionCall{
		Callee:    base,
		Arguments: args,
	}

	return expr, nil
}

func (p *Parser) parseExpression() (ast.Expr, error) {
	return p.parseBinaryExpression(OperatorPrecedenceUnset)
}

func (p *Parser) parseGroupedExpression() (ast.Expr, error) {
	expr, err := p.parseExpression()
	if err != nil {
		return nil, err
	}

	// closing
	if err := p.expectPunctuation(")"); err != nil {
		return nil, errors.New("expected ')'")
	}

	return expr, nil
}

func (p *Parser) parseIndexExpression(base ast.Expr) (ast.Expr, error) {
	token, err := p.readToken()
	if err == io.EOF {
		return nil, io.ErrUnexpectedEOF
	}
	if err != nil {
		return nil, err
	}

	var property ast.Expr
	if token.Type == lexer.TokenTypeIdentifier {
		property = &ast.Identifier{
			Name: token.Value,
		}
	}
	if token.Type == lexer.TokenTypeNumber {
		property = &ast.Literal{
			Value: token.Value,
		}
	}
	if token.Type == lexer.TokenTypeString {
		property = &ast.Literal{
			Value: token.Value,
		}
	}

	if property == nil {
		return nil, errors.New("expected identifier, number or string")
	}

	expr := &ast.IndexAccess{
		Base:     base,
		Property: property,
	}

	// closing
	if err := p.expectPunctuation("]"); err != nil {
		return nil, errors.New("expected ']'")
	}

	return expr, nil
}

func (p *Parser) parseMemberExpression(base ast.Expr) (ast.Expr, error) {
	token, err := p.readToken()
	if err == io.EOF {
		return nil, io.ErrUnexpectedEOF
	}
	if err != nil {
		return nil, err
	}

	if token.Type != lexer.TokenTypeIdentifier {
		return nil, errors.New("expected identifier")
	}

	expr := &ast.MemberAccess{
		Base: base,
		Property: &ast.Identifier{
			Name: token.Value,
		},
	}

	return expr, nil
}

func (p *Parser) parsePrimaryExpression() (ast.Expr, error) {
	token, err := p.readToken()
	if err == io.EOF {
		return nil, io.ErrUnexpectedEOF
	}
	if err != nil {
		return nil, err
	}

	var left ast.Expr
	switch {
	case token.Type == lexer.TokenTypePunctuation && token.Value == "!":
		left, err = p.parseUnaryExpression()

	case token.Type == lexer.TokenTypePunctuation && token.Value == "(":
		left, err = p.parseGroupedExpression()

	case token.Type == lexer.TokenTypeBoolean:
		value := false

		switch token.Value {
		case "true":
			value = true

		case "false":
			value = false

		default:
			panic("token type boolean has unexpected value")
		}

		left = &ast.Literal{
			Value: value,
		}

	case token.Type == lexer.TokenTypeIdentifier:
		left = &ast.Identifier{
			Name: token.Value,
		}

	case token.Type == lexer.TokenTypeNull:
		left = &ast.Literal{
			Value: ast.ValueNull,
		}

	case token.Type == lexer.TokenTypeNumber:
		integer, err := strconv.ParseInt(token.Value, 10, 64)
		if err != nil {
			panic("cannot parse value of token type number")
		}

		left = &ast.Literal{
			Value: integer,
		}

	case token.Type == lexer.TokenTypeString:
		left = &ast.Literal{
			Value: token.Value,
		}
	}

	if err == io.EOF {
		return nil, io.ErrUnexpectedEOF
	}
	if err != nil {
		return nil, err
	}

	for {
		token, err = p.readToken()
		if err == io.EOF {
			return left, nil
		}
		if err != nil {
			return nil, err
		}

		if token.Type == lexer.TokenTypePunctuation {
			// TODO: only allowed for identifiers
			if token.Value == "." {
				expr, err := p.parseMemberExpression(left)
				if err != nil {
					return nil, err
				}

				left = expr
				continue
			}

			// TODO: only allowed for identifiers
			if token.Value == "[" {
				expr, err := p.parseIndexExpression(left)
				if err != nil {
					return nil, err
				}

				left = expr
				continue
			}

			// TODO: only allowed for identifiers
			if token.Value == "(" {
				expr, err := p.parseCallExpression(left)
				if err != nil {
					return nil, err
				}

				left = expr
				continue
			}
		}

		p.unreadToken()

		return left, nil
	}
}

func (p *Parser) parseUnaryExpression() (ast.Expr, error) {
	operand, err := p.parsePrimaryExpression()
	if err != nil {
		return nil, err
	}

	expr := &ast.Unary{
		Operator: "!",
		Operand:  operand,
	}

	return expr, nil
}

func (p *Parser) peekToken() (*lexer.Token, error) {
	if p.pos >= len(p.tokens) {
		token, err := p.lexer.ReadToken()
		if err != nil {
			return nil, err
		}

		p.tokens = append(p.tokens, token)

		return token, nil
	}

	token := p.tokens[p.pos]

	return token, nil
}

func (p *Parser) readToken() (*lexer.Token, error) {
	if p.pos >= len(p.tokens) {
		token, err := p.lexer.ReadToken()
		if err != nil {
			return nil, err
		}

		p.tokens = append(p.tokens, token)
		p.pos++

		return token, nil
	}

	token := p.tokens[p.pos]
	p.pos++

	return token, nil
}

func (p *Parser) unreadToken() {
	if p.pos > 0 {
		p.pos--
	}
}

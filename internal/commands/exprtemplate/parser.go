package exprtemplate

import (
	"errors"
	"io"

	"github.com/artuross/github-actions-runner/internal/commands/exprtemplate/ast"
)

func ParseTemplate(template Template) (ast.Expr, error) {
	if template.Token == nil {
		return nil, errors.New("template is nil")
	}

	if token, ok := template.Token.(*TemplateTokenLiteral); ok && token != nil {
		expr := ast.Literal{
			Value: token.Value,
		}

		return expr, nil
	}

	if token, ok := template.Token.(*TemplateTokenExpr); ok && token != nil {
		expr := ast.Literal{
			Value: token.Expr,
		}

		return expr, nil
	}

	return nil, errors.New("unsupported token type")
}

type parser struct {
	ts     *tokenizer
	tokens []*Token
	pos    int
}

func NewParser(expr string) *parser {
	return &parser{
		ts: NewTokenizer(expr),
	}
}

func (p *parser) Parse() (ast.Expr, error) {
	return p.parseExpression()
}

func (p *parser) parseExpression() (ast.Expr, error) {
	first, err := p.readToken()
	if err != nil {
		return nil, err
	}

	if first.Type == TokenTypeKeyword {
		return p.parseKeywordExpression(first)
	}

	return nil, errors.New("unsupported token type")
}

func (p *parser) parseKeywordExpression(token *Token) (ast.Expr, error) {
	identifier := &ast.Identifier{
		Name: token.Value,
	}

	token, err := p.peekToken()
	if err == io.EOF {
		return identifier, nil
	}

	if err != nil {
		return nil, err
	}

	if token.Type == TokenTypePunctuation && token.Value == "." {
		// read and ignore the dot token
		_, _ = p.readToken()

		return p.parseMemberAccessExpression(identifier)
	}

	if token.Type == TokenTypePunctuation && token.Value == "(" {
		// read and ignore the opening parenthesis
		_, _ = p.readToken()

		return p.parseFunctionCall(identifier)
	}

	return identifier, nil
}

func (p *parser) parseFunctionCall(callee ast.Expr) (ast.Expr, error) {
	// Expect closing parenthesis
	token, err := p.readToken()
	if err != nil {
		return nil, err
	}

	if token.Type != TokenTypePunctuation || token.Value != ")" {
		return nil, errors.New("expected closing parenthesis")
	}

	expr := &ast.FunctionCall{
		Callee: callee,
	}

	// Check for chaining (.something or another function call)
	token, err = p.peekToken()
	if err == io.EOF {
		return expr, nil
	}

	if err != nil {
		return nil, err
	}

	if token.Type == TokenTypePunctuation && token.Value == "." {
		// read and ignore the dot token
		_, _ = p.readToken()
		return p.parseMemberAccessExpression(expr)
	}

	if token.Type == TokenTypePunctuation && token.Value == "(" {
		// read and ignore the opening parenthesis
		_, _ = p.readToken()
		return p.parseFunctionCall(expr)
	}

	return expr, nil
}

func (p *parser) parseMemberAccessExpression(base ast.Expr) (ast.Expr, error) {
	token, err := p.readToken()

	if err == io.EOF {
		return nil, io.ErrUnexpectedEOF
	}

	if err != nil {
		return nil, err
	}

	// after dot there must be keyword
	if token.Type != TokenTypeKeyword {
		return nil, errors.New("expected keyword token")
	}

	expr := &ast.MemberAccess{
		Base: base,
		Property: &ast.Identifier{
			Name: token.Value,
		},
	}

	token, err = p.peekToken()
	if err == io.EOF {
		return expr, nil
	}

	if err != nil {
		return nil, err
	}

	if token.Type == TokenTypePunctuation && token.Value == "." {
		// read and ignore the dot token
		_, _ = p.readToken()

		return p.parseMemberAccessExpression(expr)
	}

	return expr, nil
}

func (p *parser) readToken() (*Token, error) {
	if p.pos >= len(p.tokens) {
		token, err := p.ts.ReadToken()
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

func (p *parser) peekToken() (*Token, error) {
	if p.pos >= len(p.tokens) {
		token, err := p.ts.ReadToken()
		if err != nil {
			return nil, err
		}

		p.tokens = append(p.tokens, token)

		return token, nil
	}

	token := p.tokens[p.pos]

	return token, nil
}

func (p *parser) unreadToken() {
	if p.pos > 0 {
		p.pos--
	}
}

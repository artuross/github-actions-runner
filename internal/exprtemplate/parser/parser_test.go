package parser_test

import (
	"io"
	"testing"

	"github.com/artuross/github-actions-runner/internal/exprtemplate/ast"
	"github.com/artuross/github-actions-runner/internal/exprtemplate/lexer"
	"github.com/artuross/github-actions-runner/internal/exprtemplate/parser"
	"github.com/kr/pretty"
	"github.com/stretchr/testify/require"
)

type fakeLexer struct {
	pos    int
	tokens []*lexer.Token
}

func (l *fakeLexer) ReadToken() (*lexer.Token, error) {
	if l.pos >= len(l.tokens) {
		return nil, io.EOF
	}

	token := l.tokens[l.pos]
	l.pos += 1

	return token, nil
}

func TestParser(t *testing.T) {
	// TODO: test wildcard
	// TODO: test binary expressions
	// TODO: test unary expressions

	t.Run("literals", func(t *testing.T) {
		type testCase struct {
			name        string
			inputTokens []*lexer.Token
			outputExpr  ast.Expr
		}

		testCases := []testCase{
			{
				name: "literal / string", // 'output'
				inputTokens: []*lexer.Token{
					{
						Type:  lexer.TokenTypeString,
						Value: "output",
					},
				},
				outputExpr: &ast.Literal{
					Value: "output",
				},
			},
			{
				name: "literal / boolean", // false
				inputTokens: []*lexer.Token{
					{
						Type:  lexer.TokenTypeBoolean,
						Value: "false",
					},
				},
				outputExpr: &ast.Literal{
					Value: false,
				},
			},
			{
				name: "literal / null", // null
				inputTokens: []*lexer.Token{
					{
						Type:  lexer.TokenTypeNull,
						Value: "null",
					},
				},
				outputExpr: &ast.Literal{
					Value: ast.ValueNull,
				},
			},
			{
				name: "literal / number", // 12
				inputTokens: []*lexer.Token{
					{
						Type:  lexer.TokenTypeNumber,
						Value: "12",
					},
				},
				outputExpr: &ast.Literal{
					Value: int64(12),
				},
			},
			{
				name: "identifier", // steps
				inputTokens: []*lexer.Token{
					{
						Type:  lexer.TokenTypeIdentifier,
						Value: "steps",
					},
				},
				outputExpr: &ast.Identifier{
					Name: "steps",
				},
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				lexer := &fakeLexer{
					tokens: tc.inputTokens,
				}

				p := parser.NewParser(lexer)

				t.Log("input tokens:")
				t.Log(pretty.Sprint(tc.inputTokens))

				t.Log("expected expr:")
				t.Log(pretty.Sprint(tc.outputExpr))

				expr, err := p.Parse()
				require.NoError(t, err)

				t.Log("got expr:")
				t.Log(pretty.Sprint(expr))

				require.Equal(t, tc.outputExpr, expr)
			})
		}
	})

	t.Run("basic expressions", func(t *testing.T) {
		type testCase struct {
			name        string
			inputTokens []*lexer.Token
			outputExpr  ast.Expr
		}

		testCases := []testCase{
			{
				name: "index access", // output.steps
				inputTokens: []*lexer.Token{
					{
						Type:  lexer.TokenTypeIdentifier,
						Value: "output",
					},
					{
						Type:  lexer.TokenTypePunctuation,
						Value: ".",
					},
					{
						Type:  lexer.TokenTypeIdentifier,
						Value: "steps",
					},
				},
				outputExpr: &ast.MemberAccess{
					Base: &ast.Identifier{
						Name: "output",
					},
					Property: &ast.Identifier{
						Name: "steps",
					},
				},
			},
			{
				name: "member access", // output[steps]
				inputTokens: []*lexer.Token{
					{
						Type:  lexer.TokenTypeIdentifier,
						Value: "output",
					},
					{
						Type:  lexer.TokenTypePunctuation,
						Value: ".",
					},
					{
						Type:  lexer.TokenTypeIdentifier,
						Value: "steps",
					},
				},
				outputExpr: &ast.MemberAccess{
					Base: &ast.Identifier{
						Name: "output",
					},
					Property: &ast.Identifier{
						Name: "steps",
					},
				},
			},
			{
				name: "function call", // format()
				inputTokens: []*lexer.Token{
					{
						Type:  lexer.TokenTypeIdentifier,
						Value: "format",
					},
					{
						Type:  lexer.TokenTypePunctuation,
						Value: "(",
					},
					{
						Type:  lexer.TokenTypePunctuation,
						Value: ")",
					},
				},
				outputExpr: &ast.FunctionCall{
					Callee: &ast.Identifier{
						Name: "format",
					},
					Arguments: []ast.Expr{},
				},
			},
			{
				name: "function call with args", // format(null, 'test', output.steps)
				inputTokens: []*lexer.Token{
					{
						Type:  lexer.TokenTypeIdentifier,
						Value: "format",
					},
					{
						Type:  lexer.TokenTypePunctuation,
						Value: "(",
					},
					{
						Type:  lexer.TokenTypeNull,
						Value: "null",
					},
					{
						Type:  lexer.TokenTypePunctuation,
						Value: ",",
					},
					{
						Type:  lexer.TokenTypeString,
						Value: "test",
					},
					{
						Type:  lexer.TokenTypePunctuation,
						Value: ",",
					},
					{
						Type:  lexer.TokenTypeIdentifier,
						Value: "output",
					},
					{
						Type:  lexer.TokenTypePunctuation,
						Value: ".",
					},
					{
						Type:  lexer.TokenTypeIdentifier,
						Value: "steps",
					},
					{
						Type:  lexer.TokenTypePunctuation,
						Value: ")",
					},
				},
				outputExpr: &ast.FunctionCall{
					Callee: &ast.Identifier{
						Name: "format",
					},
					Arguments: []ast.Expr{
						&ast.Literal{
							Value: ast.ValueNull,
						},
						&ast.Literal{
							Value: "test",
						},
						&ast.MemberAccess{
							Base: &ast.Identifier{
								Name: "output",
							},
							Property: &ast.Identifier{
								Name: "steps",
							},
						},
					},
				},
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				lexer := &fakeLexer{
					tokens: tc.inputTokens,
				}

				p := parser.NewParser(lexer)

				t.Log("input tokens:")
				t.Log(pretty.Sprint(tc.inputTokens))

				t.Log("expected expr:")
				t.Log(pretty.Sprint(tc.outputExpr))

				expr, err := p.Parse()
				require.NoError(t, err)

				t.Log("got expr:")
				t.Log(pretty.Sprint(expr))

				require.Equal(t, tc.outputExpr, expr)
			})
		}
	})
}

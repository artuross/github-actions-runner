package exprtemplate_test

import (
	"testing"

	"github.com/artuross/github-actions-runner/internal/commands/exprtemplate"
	"github.com/artuross/github-actions-runner/internal/commands/exprtemplate/ast"
	"github.com/kr/pretty"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseTemplate(t *testing.T) {
	type testCase struct {
		name      string
		inputExpr string
		outputAST ast.Expr
	}

	testCases := []testCase{
		{
			name:      "member access / ab",
			inputExpr: `a.b`,
			outputAST: &ast.MemberAccess{
				Base: &ast.Identifier{
					Name: "a",
				},
				Property: &ast.Identifier{
					Name: "b",
				},
			},
		},
		{
			name:      "member access / abc",
			inputExpr: `a.b.c`,
			outputAST: &ast.MemberAccess{
				Base: &ast.MemberAccess{
					Base: &ast.Identifier{
						Name: "a",
					},
					Property: &ast.Identifier{
						Name: "b",
					},
				},
				Property: &ast.Identifier{
					Name: "c",
				},
			},
		},
		{
			name:      "function call / funcN",
			inputExpr: `a()`,
			outputAST: &ast.FunctionCall{
				Callee: &ast.Identifier{
					Name: "a",
				},
			},
		},
		{
			name:      "function call / funcNN",
			inputExpr: `a()()`,
			outputAST: &ast.FunctionCall{
				Callee: &ast.FunctionCall{
					Callee: &ast.Identifier{
						Name: "a",
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			parser := exprtemplate.NewParser(tc.inputExpr)

			expr, err := parser.Parse()

			t.Log(pretty.Sprint(expr))

			require.NoError(t, err)
			assert.Equal(t, tc.outputAST, expr)
		})
	}
}

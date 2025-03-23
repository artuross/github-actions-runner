package evaluate_test

import (
	"fmt"
	"testing"

	"github.com/artuross/github-actions-runner/internal/exprtemplate/evaluate"
	"github.com/artuross/github-actions-runner/internal/exprtemplate/lexer"
	"github.com/artuross/github-actions-runner/internal/exprtemplate/parser"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEvaluator(t *testing.T) {
	type testCase struct {
		inputExpr     string
		expectedValue any
		expectedError error
	}

	testCases := []testCase{
		{
			inputExpr:     "1",
			expectedValue: "1",
			expectedError: nil,
		},
		{
			inputExpr:     "1 || 2",
			expectedValue: "1",
			expectedError: nil,
		},
		{
			inputExpr:     "0 || 1",
			expectedValue: "1",
			expectedError: nil,
		},
	}

	for index, testCase := range testCases {
		t.Run(fmt.Sprintf("test %02d", index), func(t *testing.T) {
			p := parser.NewParser(lexer.NewLexer(testCase.inputExpr))

			ast, err := p.Parse()
			require.NoError(t, err)

			eval := evaluate.New(ast)

			value, err := eval.Evaluate(evaluate.Context{})
			require.NoError(t, err)

			assert.Equal(t, testCase.expectedValue, value)
		})
	}
}

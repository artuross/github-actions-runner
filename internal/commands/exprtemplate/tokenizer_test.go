package exprtemplate_test

import (
	"io"
	"testing"

	"github.com/artuross/github-actions-runner/internal/commands/exprtemplate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTokenizer(t *testing.T) {
	type testCase struct {
		name   string
		input  string
		tokens []*exprtemplate.Token
	}

	t.Run("single token", func(t *testing.T) {
		newTestCase := func(name string, input string, expectedTokenType exprtemplate.TokenType, expectedValue string) testCase {
			return testCase{
				name:  name,
				input: input,
				tokens: []*exprtemplate.Token{
					{
						Type:  expectedTokenType,
						Value: expectedValue,
					},
				},
			}
		}

		testCases := []testCase{
			newTestCase("keyword", "github", exprtemplate.TokenTypeKeyword, "github"),
			newTestCase("keyword / underscore", "_github", exprtemplate.TokenTypeKeyword, "_github"),
			{
				name:  "number",
				input: "12",
				tokens: []*exprtemplate.Token{
					{
						Type:  exprtemplate.TokenTypeNumber,
						Value: "12",
					},
				},
			},
			{
				name:  "number / positive",
				input: "+12.1",
				tokens: []*exprtemplate.Token{
					{
						Type:  exprtemplate.TokenTypeNumber,
						Value: "+12.1",
					},
				},
			},
			{
				name:  "number / negative",
				input: "-12.1",
				tokens: []*exprtemplate.Token{
					{
						Type:  exprtemplate.TokenTypeNumber,
						Value: "-12.1",
					},
				},
			},
			{
				name:  "number / dot",
				input: ".1",
				tokens: []*exprtemplate.Token{
					{
						Type:  exprtemplate.TokenTypeNumber,
						Value: ".1",
					},
				},
			},
			{
				name:  "string",
				input: `'hello'`,
				tokens: []*exprtemplate.Token{
					{
						Type:  exprtemplate.TokenTypeString,
						Value: "hello",
					},
				},
			},
			{
				name:  "string / apostrophe",
				input: `'hel\'lo'`,
				tokens: []*exprtemplate.Token{
					{
						Type:  exprtemplate.TokenTypeString,
						Value: "hel'lo",
					},
				},
			},
			{
				name:  "string / \\",
				input: `'hel\\lo'`,
				tokens: []*exprtemplate.Token{
					{
						Type:  exprtemplate.TokenTypeString,
						Value: "hel\\lo",
					},
				},
			},
			{
				name:  "punctuation / negation",
				input: "!",
				tokens: []*exprtemplate.Token{
					{
						Type:  exprtemplate.TokenTypePunctuation,
						Value: "!",
					},
				},
			},
			{
				name:  "punctuation / lparen",
				input: "(",
				tokens: []*exprtemplate.Token{
					{
						Type:  exprtemplate.TokenTypePunctuation,
						Value: "(",
					},
				},
			},
			{
				name:  "punctuation / rparen",
				input: ")",
				tokens: []*exprtemplate.Token{
					{
						Type:  exprtemplate.TokenTypePunctuation,
						Value: ")",
					},
				},
			},
			{
				name:  "punctuation / comma",
				input: ",",
				tokens: []*exprtemplate.Token{
					{
						Type:  exprtemplate.TokenTypePunctuation,
						Value: ",",
					},
				},
			},
			{
				name:  "punctuation / dot",
				input: ".",
				tokens: []*exprtemplate.Token{
					{
						Type:  exprtemplate.TokenTypePunctuation,
						Value: ".",
					},
				},
			},
			{
				name:  "wildcard",
				input: "*",
				tokens: []*exprtemplate.Token{
					{
						Type:  exprtemplate.TokenTypeWildcard,
						Value: "*",
					},
				},
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				tokenizer := exprtemplate.NewTokenizer(tc.input)

				tokens := make([]*exprtemplate.Token, 0)

				for {
					token, err := tokenizer.ReadToken()
					if err == io.EOF {
						break
					}

					require.NoError(t, err)

					tokens = append(tokens, token)
				}

				assert.Equal(t, tc.tokens, tokens)
			})
		}
	})
}

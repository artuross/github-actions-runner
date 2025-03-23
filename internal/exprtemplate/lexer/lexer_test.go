package lexer_test

import (
	"fmt"
	"io"
	"testing"

	"github.com/artuross/github-actions-runner/internal/exprtemplate/lexer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLexer(t *testing.T) {
	t.Run("punctuation", func(t *testing.T) {
		values := []string{
			"!", "(", ")", ",", ".", "<", ">", "[", "]", // single char
			"!=", "&&", "<=", "==", ">=", "||", // double char
		}

		for index, value := range values {
			t.Run(fmt.Sprintf("%d - %s", index, value), func(t *testing.T) {
				expectedToken := &lexer.Token{
					Type:     lexer.TokenTypePunctuation,
					Position: position(1, 1, 1, len(value)+1),
					RawValue: value,
					Value:    value,
				}

				lex := lexer.NewLexer(value)

				tokens := make([]*lexer.Token, 0)

				index := 0
				for {
					token, err := lex.ReadToken()
					if err == io.EOF {
						break
					}
					require.NoError(t, err, "error when reading token %d", index)

					tokens = append(tokens, token)

					index++
				}

				t.Logf("expression: %v", value)

				require.Equal(t, 1, len(tokens), "incorrect number of tokens")

				assert.Equal(t, expectedToken, tokens[0])
			})
		}
	})

	t.Run("remaining", func(t *testing.T) {
		type testCase struct {
			name   string
			input  string
			tokens []*lexer.Token
		}

		testCases := []testCase{
			{
				name:  "boolean / false",
				input: "false",
				tokens: []*lexer.Token{
					{
						Type:     lexer.TokenTypeBoolean,
						Position: position(1, 1, 1, 6),
						RawValue: "false",
						Value:    "false",
					},
				},
			},
			{
				name:  "boolean / true",
				input: "true",
				tokens: []*lexer.Token{
					{
						Type:     lexer.TokenTypeBoolean,
						Position: position(1, 1, 1, 5),
						RawValue: "true",
						Value:    "true",
					},
				},
			},
			{
				name:  "identifier",
				input: "a",
				tokens: []*lexer.Token{
					{
						Type:     lexer.TokenTypeIdentifier,
						Position: position(1, 1, 1, 2),
						RawValue: "a",
						Value:    "a",
					},
				},
			},
			// {
			// 	name:  "identifier / handles spaces",
			// 	input: " a b ",
			// 	tokens: []*lexer.Token{
			// 		{
			// 			Type:     lexer.TokenTypeIdentifier,
			// 			Position: position(1, 2, 1, 3),
			// 			RawValue: "a",
			// 			Value:    "a",
			// 		},
			// 		{
			// 			Type:     lexer.TokenTypeIdentifier,
			// 			Position: position(1, 4, 1, 5),
			// 			RawValue: "b",
			// 			Value:    "b",
			// 		},
			// 	},
			// },
			{
				name:  "null",
				input: "null",
				tokens: []*lexer.Token{
					{
						Type:     lexer.TokenTypeNull,
						Position: position(1, 1, 1, 5),
						RawValue: "null",
						Value:    "null",
					},
				},
			},
			{
				name:  "number / integer",
				input: "123",
				tokens: []*lexer.Token{
					{
						Type:     lexer.TokenTypeNumber,
						Position: position(1, 1, 1, 4),
						RawValue: "123",
						Value:    "123",
					},
				},
			},
			{
				name:  "string",
				input: "'test'",
				tokens: []*lexer.Token{
					{
						Type:     lexer.TokenTypeString,
						Position: position(1, 1, 1, 7),
						RawValue: "'test'",
						Value:    "test",
					},
				},
			},
			{
				name:  "wildcards",
				input: "**",
				tokens: []*lexer.Token{
					{
						Type:     lexer.TokenTypeWildcard,
						Position: position(1, 1, 1, 2),
						RawValue: "*",
						Value:    "*",
					},
					{
						Type:     lexer.TokenTypeWildcard,
						Position: position(1, 2, 1, 3),
						RawValue: "*",
						Value:    "*",
					},
				},
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				lex := lexer.NewLexer(tc.input)

				tokens := make([]*lexer.Token, 0)

				index := 0
				for {
					token, err := lex.ReadToken()
					if err == io.EOF {
						break
					}
					require.NoError(t, err, "error when reading token %d", index)

					tokens = append(tokens, token)

					index++
				}

				t.Logf("expression: %v", tc.input)

				require.Equal(t, len(tc.tokens), len(tokens), "incorrect number of tokens")

				for i := range len(tc.tokens) {
					assert.Equal(t, tc.tokens[i], tokens[i], "token index %d", i)
				}
			})
		}
	})
}

func position(startLine, startColumnt, endLine, endColumnt int) lexer.Position {
	return lexer.Position{
		Start: lexer.Point{
			Line:   startLine,
			Column: startColumnt,
		},
		End: lexer.Point{
			Line:   endLine,
			Column: endColumnt,
		},
	}
}

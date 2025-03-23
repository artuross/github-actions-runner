package lexer

import (
	"errors"
	"io"
	"slices"
	"unicode/utf8"
)

type TokenType string

const (
	TokenTypeBoolean     TokenType = "BOOLEAN"
	TokenTypeIdentifier  TokenType = "IDENTIFIER"
	TokenTypeNull        TokenType = "NULL"
	TokenTypeNumber      TokenType = "NUMBER"
	TokenTypePunctuation TokenType = "PUNCTUATION"
	TokenTypeString      TokenType = "STRING"
	TokenTypeWildcard    TokenType = "WILDCARD"
)

var (
	ErrInvalidCharacter   = errors.New("invalid character")
	ErrInvalidPunctuation = errors.New("invalid punctuation")
	ErrRuneInvalid        = errors.New("decode rune: invalid rune")
)

type Point struct {
	Line   int
	Column int
}

type Position struct {
	Start Point
	End   Point
}

type Token struct {
	Type     TokenType
	RawValue string
	Value    string
	Position Position
}

type Lexer struct {
	input    []byte
	point    Point
	position int
}

func NewLexer(input string) *Lexer {
	return &Lexer{
		input:    []byte(input),
		point:    Point{Line: 1, Column: 1},
		position: 0,
	}
}

func (l *Lexer) ReadToken() (*Token, error) {
	if err := l.advanceWhitespace(); err != nil {
		return nil, err
	}

	r, _, err := l.peek()
	if err == io.EOF {
		return nil, io.EOF
	}
	if err != nil {
		return nil, err
	}

	// TODO: add support for decimal numbers, symbols and other variants
	if isDigit(r) {
		return l.readNumber()
	}

	if isPunctuationOpeningCharacter(r) {
		return l.readPunctuation()
	}

	if isIdentifierOpeningCharacter(r) {
		return l.readIdentifier()
	}

	if isStringOpeningCharacter(r) {
		return l.readString()
	}

	if r == '*' {
		return l.readWildcard()
	}

	return nil, ErrInvalidCharacter
}

func (l *Lexer) advanceWhitespace() error {
	for {
		r, _, err := l.peek()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}

		switch r {
		case ' ', '\t', '\r':
			// TODO: should panic here on err?
			// position and column updated inside read call
			_, _ = l.read()

		case '\n':
			// TODO: should panic here on err?
			_, _ = l.read()

			l.point.Line++
			l.point.Column = 1

		default:
			return nil
		}
	}
}

func (l *Lexer) readIdentifier() (*Token, error) {
	startPoint := l.point

	r, err := l.read()
	invariant(err != nil, "readIdentifier: unexpected read() error when consuming first character")
	invariant(!isIdentifierOpeningCharacter(r), "readIdentifier: first character is not valid")

	value := []rune{r}

	for {
		r, _, err := l.peek()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		if !isIdentifierContinuationCharacter(r) {
			break
		}

		_, err = l.read()
		invariant(err != nil, "readIdentifier: unexpected read() error after peek()")

		value = append(value, r)
	}

	endPoint := l.point

	tokenType := TokenTypeIdentifier
	if string(value) == "null" {
		tokenType = TokenTypeNull
	} else if slices.Contains([]string{"false", "true"}, string(value)) {
		tokenType = TokenTypeBoolean
	}

	token := Token{
		Type: tokenType,
		Position: Position{
			Start: startPoint,
			End:   endPoint,
		},
		RawValue: string(value),
		Value:    string(value),
	}

	return &token, nil
}

func (l *Lexer) readNumber() (*Token, error) {
	startPoint := l.point
	startPos := l.position

	value := make([]rune, 0, 1)

	// read first rune, we know it belongs to the number
	r, err := l.read()
	invariant(err != nil, "readNumber: unexpected read() error when consuming first character")

	if isDigit(r) {
		value = append(value, r)
	}

	for {
		r, _, err := l.peek()
		if err == io.EOF {
			break
		}

		if !isDigit(r) {
			break
		}

		_, err = l.read()
		invariant(err != nil, "readNumber: unexpected read() error after peek()")

		value = append(value, r)
	}

	endPoint := l.point
	endPos := l.position

	token := Token{
		Type: TokenTypeNumber,
		Position: Position{
			Start: startPoint,
			End:   endPoint,
		},
		RawValue: string(l.input[startPos:endPos]),
		Value:    string(value),
	}

	return &token, nil
}

func (t *Lexer) readPunctuation() (*Token, error) {
	startPoint := t.point
	startPos := t.position

	newToken := func(value string) (*Token, error) {
		endPoint := t.point
		endPos := t.position

		token := Token{
			Type: TokenTypePunctuation,
			Position: Position{
				Start: startPoint,
				End:   endPoint,
			},
			RawValue: string(t.input[startPos:endPos]),
			Value:    string(value),
		}

		return &token, nil
	}

	first, err := t.read()
	invariant(err != nil, "readPunctuation: unexpected read() error when consuming first character")

	second, _, err := t.peek()
	if err == io.EOF {
		// test 1 char punctuation
		value := string(first)
		if isValidPunctuation(value) {
			return newToken(value)
		}

		return nil, ErrInvalidPunctuation
	}

	if err != nil {
		return nil, err
	}

	// test 2 char punctuation
	if value := string(first) + string(second); isValidPunctuation(value) {
		_, err := t.read()
		invariant(err != nil, "readPunctuation: unexpected read() error after peek()")

		return newToken(value)
	}

	// test 1 char punctuation
	if value := string(first); isValidPunctuation(value) {
		return newToken(value)
	}

	return nil, ErrInvalidPunctuation
}

func (l *Lexer) readString() (*Token, error) {
	startPoint := l.point
	startPos := l.position

	// discard the value
	r, err := l.read()
	invariant(err != nil, "readString: unexpected read() error when consuming first character")
	invariant(!isStringOpeningCharacter(r), "readString: first character is not valid")

	value := []rune{}

	for {
		r, _, err := l.peek()
		if err == io.EOF {
			return nil, io.ErrUnexpectedEOF
		}
		if err != nil {
			return nil, err
		}

		// TODO: add escape character handling
		if r == '\'' {
			_, err := l.read()
			invariant(err != nil, "readString: unexpected read() error after peek()")

			break
		}

		_, err = l.read()
		invariant(err != nil, "readString: unexpected read() error after peek()")

		value = append(value, r)
	}

	endPoint := l.point
	endPos := l.position

	token := Token{
		Type: TokenTypeString,
		Position: Position{
			Start: startPoint,
			End:   endPoint,
		},
		RawValue: string(l.input[startPos:endPos]),
		Value:    string(value),
	}

	return &token, nil
}

func (l *Lexer) readWildcard() (*Token, error) {
	startPoint := l.point

	// discard rune, we know it's wildcard
	_, err := l.read()
	invariant(err != nil, "readWildcard: unexpected read() error when consuming first character")

	endPoint := l.point

	token := Token{
		Type: TokenTypeWildcard,
		Position: Position{
			Start: startPoint,
			End:   endPoint,
		},
		RawValue: "*",
		Value:    "*",
	}

	return &token, nil
}

func (l *Lexer) peek() (rune, int, error) {
	if l.position >= len(l.input) {
		return 0, 0, io.EOF
	}

	r, size := utf8.DecodeRune(l.input[l.position:])
	if r == utf8.RuneError {
		invariant(size == 0, "peek() called on an empty slice")

		return 0, 0, ErrRuneInvalid
	}

	return r, size, nil
}

func (l *Lexer) read() (rune, error) {
	r, size, err := l.peek()
	if err != nil {
		return 0, err
	}

	l.position += size
	l.point.Column += size

	return r, nil
}

func isDigit(r rune) bool {
	return (r >= '0' && r <= '9')
}

func isLetter(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}

func isIdentifierContinuationCharacter(r rune) bool {
	return isIdentifierOpeningCharacter(r) || isDigit(r) || r == '-'
}

func isIdentifierOpeningCharacter(r rune) bool {
	return isLetter(r) || r == '_'
}

func isStringOpeningCharacter(r rune) bool {
	return r == '\''
}

func isPunctuationOpeningCharacter(r rune) bool {
	return slices.Contains([]rune{'!', '(', ')', ',', '.', '<', '>', '[', ']', '&', '=', '|', ','}, r)
}

func isValidPunctuation(value string) bool {
	return slices.Contains(
		[]string{
			"!", "(", ")", ",", ".", "<", ">", "[", "]", ",", // single char
			"!=", "&&", "<=", "==", ">=", "||", // double char
		},
		value,
	)
}

func invariant(assertion bool, message string) {
	if assertion {
		panic(message)
	}
}

package exprtemplate

import (
	"errors"
	"io"
	"slices"
	"unicode/utf8"
)

var (
	ErrExpectedStringEnd     = errors.New("expected string end")
	ErrUnexpectedPunctuation = errors.New("unexpected punctuation")
	ErrUnknownToken          = errors.New("unknown token")
)

type TokenType string

const (
	TokenTypeKeyword     TokenType = "keyword"
	TokenTypeNumber      TokenType = "number"
	TokenTypePunctuation TokenType = "punctuation"
	TokenTypeString      TokenType = "string"
	TokenTypeWildcard    TokenType = "wildcard"
)

type Token struct {
	Type  TokenType
	Value string
}

type tokenizer struct {
	expr []byte
	pos  int
}

func NewTokenizer(expr string) *tokenizer {
	return &tokenizer{
		expr: []byte(expr),
		pos:  0,
	}
}

func (t *tokenizer) ReadToken() (*Token, error) {
	r, err := t.peek()
	if err != nil {
		return nil, err
	}

	if r == '.' {
		next, err := t.peekN(2)
		if err == io.EOF {
			return t.readPunctuation()
		}

		if err != nil {
			return nil, err
		}

		if isDigit(next) {
			return t.readNumber()
		}

		return t.readPunctuation()
	}

	if slices.Contains([]rune{'!', '&', '(', ')', ',', '<', '=', '>', '[', ']', '|'}, r) {
		return t.readPunctuation()
	}

	if isKeywordOpeningCharacter(r) {
		return t.readKeyword()
	}

	if isNumberOpeningCharacter(r) {
		return t.readNumber()
	}

	if isStringOpeningCharacter(r) {
		return t.readString()
	}

	if r == '*' {
		return t.readWildcard()
	}

	return nil, ErrUnknownToken
}

func (t *tokenizer) readKeyword() (*Token, error) {
	keyword := ""

	// read first rune, we know it belongs to the keyword
	r, err := t.next()
	if err != nil {
		return nil, err
	}
	keyword += string(r)

	for {
		r, err := t.next()
		if err == io.EOF {
			break
		}

		if err != nil {
			return nil, err
		}

		if !isKeywordContinuationCharacter(r) {
			t.unread()
			break
		}

		keyword += string(r)
	}

	token := &Token{
		Type:  TokenTypeKeyword,
		Value: keyword,
	}

	return token, nil
}

func (t *tokenizer) readNumber() (*Token, error) {
	number := ""

	// read first rune, we know it belongs to the number
	r, err := t.next()
	if err != nil {
		return nil, err
	}
	number += string(r)

	// whether we already read a dot "."
	readDelimiter := r == '.'

	for {
		r, err := t.next()
		if err == io.EOF {
			break
		}

		if err != nil {
			return nil, err
		}

		if !isNumberContinuationCharacter(r) {
			t.unread()
			break
		}

		// TODO: after . there must be a digit
		// TODO: after += there must be either a dot or a digit
		if readDelimiter && r == '.' {
			t.unread()
			break
		}

		if r == '.' {
			readDelimiter = true
		}

		number += string(r)
	}

	token := &Token{
		Type:  TokenTypeNumber,
		Value: number,
	}

	return token, nil
}

func (t *tokenizer) readPunctuation() (*Token, error) {
	first, err := t.next()
	if err != nil {
		return nil, err
	}

	oneCharPunct := ""
	switch first {
	case '!':
		oneCharPunct = "!"

	case '(':
		oneCharPunct = "("

	case ')':
		oneCharPunct = ")"

	case ',':
		oneCharPunct = ","

	case '.':
		oneCharPunct = "."

	case '<':
		oneCharPunct = "<"

	case '>':
		oneCharPunct = ">"

	case '[':
		oneCharPunct = "["

	case ']':
		oneCharPunct = "]"

	default:
		return nil, ErrUnexpectedPunctuation
	}

	second, err := t.next()
	if err == io.EOF {
		token := Token{
			Type:  TokenTypePunctuation,
			Value: oneCharPunct,
		}

		return &token, nil
	}

	if err != nil {
		return nil, err
	}

	twoCharPunct := ""

	switch {
	case first == '!' && second == '=':
		twoCharPunct = "!="

	case first == '&' && second == '&':
		twoCharPunct = "&&"

	case first == '<' && second == '=':
		twoCharPunct = "<="

	case first == '=' && second == '=':
		twoCharPunct = "=="

	case first == '>' && second == '=':
		twoCharPunct = ">="

	case first == '|' && second == '|':
		twoCharPunct = "||"

	default:
		// char is not part of a two-char punctuation
		t.unread()

		token := Token{
			Type:  TokenTypePunctuation,
			Value: oneCharPunct,
		}

		return &token, nil
	}

	token := Token{
		Type:  TokenTypePunctuation,
		Value: twoCharPunct,
	}

	return &token, nil
}

func (t *tokenizer) readString() (*Token, error) {
	value := ""

	// discard it, we know it belongs to the string
	_, err := t.next()
	if err != nil {
		return nil, err
	}

	for {
		r, err := t.next()
		if err == io.EOF {
			return nil, ErrExpectedStringEnd
		}

		if err != nil {
			return nil, err
		}

		if r == '\\' {
			p, err := t.next()
			if err == io.EOF {
				return nil, ErrExpectedStringEnd
			}

			if err != nil {
				return nil, err
			}

			// check if next character is ' and consume it
			if p == '\'' {
				value += string(p)
				continue
			}

			// check if next character is \ and consume it
			if p == '\\' {
				value += string(p)
				continue
			}

			// otherwise add both
			value += string(r)
			value += string(p)
			continue
		}

		if r == '\'' {
			break
		}

		value += string(r)
	}

	token := &Token{
		Type:  TokenTypeString,
		Value: value,
	}

	return token, nil
}

func (t *tokenizer) readWildcard() (*Token, error) {
	// discard rune, we know it's wildcard
	_, err := t.next()
	if err != nil {
		return nil, err
	}

	token := Token{
		Type:  TokenTypeWildcard,
		Value: "*",
	}

	return &token, nil
}

func (t *tokenizer) peek() (rune, error) {
	if t.pos >= len(t.expr) {
		return 0, io.EOF
	}

	r, _ := utf8.DecodeRune(t.expr[t.pos:])

	return r, nil
}

func (t *tokenizer) peekN(n int) (rune, error) {
	var lastRune rune

	pos := t.pos
	for n > 0 {
		n--

		if pos >= len(t.expr) {
			return 0, io.EOF
		}

		r, size := utf8.DecodeRune(t.expr[pos:])

		pos += size
		lastRune = r
	}

	return lastRune, nil
}

func (t *tokenizer) next() (rune, error) {
	if t.pos >= len(t.expr) {
		return 0, io.EOF
	}

	r, size := utf8.DecodeRune(t.expr[t.pos:])
	t.pos += size

	return r, nil
}

func (t *tokenizer) unread() {
	_, size := utf8.DecodeLastRune(t.expr[:t.pos])
	t.pos -= size
}

func isDigit(r rune) bool {
	return (r >= '0' && r <= '9')
}

func isLetter(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}

func isKeywordContinuationCharacter(r rune) bool {
	return isKeywordOpeningCharacter(r) || isDigit(r) || r == '-'
}

func isKeywordOpeningCharacter(r rune) bool {
	return isLetter(r) || r == '_'
}

func isNumberContinuationCharacter(r rune) bool {
	return r == '.' || isDigit(r)
}

func isNumberOpeningCharacter(r rune) bool {
	return r == '.' || r == '-' || r == '+' || isDigit(r)
}

func isStringOpeningCharacter(r rune) bool {
	return r == '\''
}

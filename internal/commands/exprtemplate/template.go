package exprtemplate

import (
	"encoding/json"
	"fmt"
)

type Location struct {
	FileIndex int
	Line      int
	Col       int
}

type TemplateToken interface {
	Location() *Location
}

type (
	TemplateTokenExpr struct {
		location Location
		Expr     string
	}

	TemplateTokenLiteral struct {
		location Location
		Value    string
	}

	TemplateTokenMapValue struct {
		Key   string
		Token TemplateToken
	}
)

type TemplateTokenMap struct {
	Values []TemplateTokenMapValue
}

type Template struct {
	Token TemplateToken
}

func (t TemplateTokenExpr) Location() *Location    { return &t.location }
func (t TemplateTokenLiteral) Location() *Location { return &t.location }
func (t TemplateTokenMap) Location() *Location     { return nil }

type TemplateTokenType int

const (
	TokenTypeExpr    TemplateTokenType = 3
	TokenTypeLiteral TemplateTokenType = 0
	TokenTypeMap     TemplateTokenType = 2
)

type jsonToken struct {
	Type      TemplateTokenType `json:"type"`
	FileIndex int               `json:"file,omitempty"`
	Line      int               `json:"line,omitempty"`
	Col       int               `json:"col,omitempty"`
	Expr      string            `json:"expr,omitempty"`
	Literal   string            `json:"lit,omitempty"`
	Map       []jsonMapItem     `json:"map,omitempty"`
}

type jsonMapItem struct {
	Key   string    `json:"key"`
	Value jsonToken `json:"value"`
}

func (t *Template) UnmarshalJSON(data []byte) error {
	var jt jsonToken
	if err := json.Unmarshal(data, &jt); err != nil {
		return err
	}

	token, err := parseJsonToken(jt)
	if err != nil {
		return err
	}

	t.Token = token

	return nil
}

func parseJsonToken(jt jsonToken) (TemplateToken, error) {
	switch jt.Type {
	case TokenTypeExpr:
		loc := Location{
			FileIndex: jt.FileIndex,
			Line:      jt.Line,
			Col:       jt.Col,
		}

		tok := TemplateTokenExpr{
			location: loc,
			Expr:     jt.Expr,
		}

		return &tok, nil

	case TokenTypeLiteral:
		loc := Location{
			FileIndex: jt.FileIndex,
			Line:      jt.Line,
			Col:       jt.Col,
		}

		tok := TemplateTokenLiteral{
			location: loc,
			Value:    jt.Literal,
		}

		return &tok, nil

	case TokenTypeMap:
		values := make([]TemplateTokenMapValue, 0, len(jt.Map))

		for _, item := range jt.Map {
			token, err := parseJsonToken(item.Value)
			if err != nil {
				return nil, err
			}

			kv := TemplateTokenMapValue{
				Key:   item.Key,
				Token: token,
			}

			values = append(values, kv)
		}

		tok := TemplateTokenMap{
			Values: values,
		}

		return &tok, nil

	default:
		return nil, fmt.Errorf("unknown token type: %d", jt.Type)
	}
}

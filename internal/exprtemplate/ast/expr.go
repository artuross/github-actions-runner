package ast

var (
	_ Expr = (*Binary)(nil)
	_ Expr = (*FunctionCall)(nil)
	_ Expr = (*Identifier)(nil)
	_ Expr = (*IndexAccess)(nil)
	_ Expr = (*Literal)(nil)
	_ Expr = (*MemberAccess)(nil)
	_ Expr = (*Unary)(nil)
)

type Expr interface {
	isExpr()
}

type (
	Binary struct {
		Left     Expr
		Operator string
		Right    Expr
	}

	FunctionCall struct {
		Callee    Expr
		Arguments []Expr
	}

	Identifier struct {
		Name string
	}

	IndexAccess struct {
		Base     Expr
		Property Expr
	}

	Literal struct {
		Value any
	}

	MemberAccess struct {
		Base     Expr
		Property *Identifier
	}

	Unary struct {
		Operator string
		Operand  Expr
	}
)

func (e Binary) isExpr()       {}
func (e FunctionCall) isExpr() {}
func (e Identifier) isExpr()   {}
func (e IndexAccess) isExpr()  {}
func (e Literal) isExpr()      {}
func (e MemberAccess) isExpr() {}
func (e Unary) isExpr()        {}

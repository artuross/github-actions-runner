package ast

type Expr any

type (
	FunctionCall struct {
		Callee Expr
	}

	Identifier struct {
		Name string
	}

	Literal struct {
		Value string
	}

	MemberAccess struct {
		Base     Expr
		Property *Identifier
	}
)

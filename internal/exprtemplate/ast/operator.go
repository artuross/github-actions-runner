package ast

type Operator string

const (
	OperatorOr  Operator = "||"
	OperatorAnd Operator = "&&"

	OperatorEqual    Operator = "=="
	OperatorNotEqual Operator = "!="

	OperatorGreaterThan        Operator = ">"
	OperatorGreaterThanOrEqual Operator = ">="
	OperatorLessThan           Operator = "<"
	OperatorLessThanOrEqual    Operator = "<="

	OperatorNot Operator = "!"
)

package evaluate

import (
	"errors"
	"fmt"

	"github.com/artuross/github-actions-runner/internal/exprtemplate/ast"
)

var (
	ErrMissingVariable   = errors.New("missing variable")
	ErrUndefinedFunction = errors.New("function is not defined")
)

var Null = (*struct{})(nil)

type Function func(...any) (any, error)

type Context struct {
	Variables map[string]any
	Functions map[string]Function
}

type Evaluator struct {
	Expr ast.Expr
}

func New(expr ast.Expr) *Evaluator {
	return &Evaluator{
		Expr: expr,
	}
}

func (e *Evaluator) Evaluate(evalContext Context) (any, error) {
	return e.evaluate(evalContext, e.Expr)
}

func (e *Evaluator) evaluate(evalContext Context, expr ast.Expr) (any, error) {
	switch expr := expr.(type) {
	case *ast.Binary:
		return e.evaluateBinary(evalContext, expr)

	case *ast.FunctionCall:
		return e.evaluateFunctionCall(evalContext, expr)

	case *ast.Identifier:
		return e.evaluateIdentifier(evalContext, expr)

	case *ast.IndexAccess:
		return e.evaluateIndexAccess(evalContext, expr)

	case *ast.Literal:
		return e.evaluateLiteral(evalContext, expr)

	case ast.MemberAccess:
		return e.evaluateMemberAccess(evalContext, expr)

	case *ast.Unary:
		return e.evaluateUnary(evalContext, expr)

	default:
		return nil, fmt.Errorf("unsupported expression type: %T", expr)
	}
}

func (e *Evaluator) evaluateBinary(evalContext Context, expr *ast.Binary) (any, error) {
	switch ast.Operator(expr.Operator) {
	case ast.OperatorOr:
		leftValue, err := e.evaluate(evalContext, expr.Left)
		if err != nil {
			return nil, err
		}

		if convertToBoolean(leftValue) {
			return leftValue, nil
		}

		rightValue, err := e.evaluate(evalContext, expr.Right)
		if err != nil {
			return nil, err
		}

		return rightValue, nil

	case ast.OperatorAnd:
		leftValue, err := e.evaluate(evalContext, expr.Left)
		if err != nil {
			return nil, err
		}

		if !convertToBoolean(leftValue) {
			return leftValue, nil
		}

		rightValue, err := e.evaluate(evalContext, expr.Right)
		if err != nil {
			return nil, err
		}

		return rightValue, nil

	case ast.OperatorEqual:
		leftValue, err := e.evaluate(evalContext, expr.Left)
		if err != nil {
			return nil, err
		}

		rightValue, err := e.evaluate(evalContext, expr.Right)
		if err != nil {
			return nil, err
		}

		value := leftValue == rightValue

		return value, nil

	case ast.OperatorNotEqual:
		leftValue, err := e.evaluate(evalContext, expr.Left)
		if err != nil {
			return nil, err
		}

		rightValue, err := e.evaluate(evalContext, expr.Right)
		if err != nil {
			return nil, err
		}

		value := leftValue != rightValue

		return value, nil

	case ast.OperatorGreaterThan:
		panic("not implemented")

	case ast.OperatorGreaterThanOrEqual:
		panic("not implemented")

	case ast.OperatorLessThan:
		panic("not implemented")

	case ast.OperatorLessThanOrEqual:
		panic("not implemented")

	}

	return nil, errors.New("not supported")
}

func (e *Evaluator) evaluateFunctionCall(evalContext Context, expr *ast.FunctionCall) (any, error) {
	funcName := ""

	switch identifier := expr.Callee.(type) {
	case *ast.Identifier:
		funcName = identifier.Name

	default:
		value, err := e.evaluate(evalContext, expr.Callee)
		if err != nil {
			return nil, err
		}

		val, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("invalid function name")
		}

		funcName = val
	}

	if funcName == "" {
		return nil, fmt.Errorf("function name may not be empty")
	}

	fn, ok := evalContext.Functions[funcName]
	if !ok {
		return nil, fmt.Errorf("function %s not found", funcName)
	}

	value, err := fn(evalContext)
	if err != nil {
		return nil, err
	}

	return value, nil
}

func (e *Evaluator) evaluateIdentifier(evalContext Context, expr *ast.Identifier) (any, error) {
	value, ok := evalContext.Variables[expr.Name]
	if !ok {
		return nil, fmt.Errorf("value not found: %s", expr.Name)
	}

	return value, nil
}

func (e *Evaluator) evaluateIndexAccess(evalContext Context, expr *ast.IndexAccess) (any, error) {
	return nil, errors.New("not supported")
}

func (e *Evaluator) evaluateLiteral(evalContext Context, expr *ast.Literal) (any, error) {
	return expr.Value, nil
}

func (e *Evaluator) evaluateMemberAccess(evalContext Context, expr ast.MemberAccess) (any, error) {
	baseRaw, err := e.evaluate(evalContext, expr.Base)
	if err != nil {
		return nil, err
	}

	base, ok := baseRaw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("cannot access member of non-object type")
	}

	propertyRaw, err := e.evaluateIdentifier(evalContext, expr.Property)
	if err != nil {
		return nil, err
	}

	property, ok := propertyRaw.(string)
	if !ok {
		return nil, fmt.Errorf("property must be a string")
	}

	value, ok := base[property]
	if !ok {
		return Null, nil
	}

	return value, nil
}

func (e *Evaluator) evaluateUnary(evalContext Context, expr *ast.Unary) (any, error) {
	if ast.Operator(expr.Operator) == ast.OperatorNot {
		raw, err := e.evaluate(evalContext, expr.Operand)
		if err != nil {
			return nil, err
		}

		// convert to bool and negate
		value := !convertToBoolean(raw)

		return value, nil

	}

	return nil, errors.New("not supported")
}

func convertToBoolean(value any) bool {
	if value == nil {
		return false
	}

	switch v := value.(type) {
	case bool:
		return v

	// TODO: remove int, we support int64 only
	case int, int64:
		return v != 0

	// TODO: remove float32, we support float64 only
	case float32, float64:
		return v != 0.0

	case string:
		return v != ""

	// slices are always of this type
	case []any:
		return len(v) > 0

	// maps are always of this type
	case map[string]any:
		return len(v) > 0

	default:
		return false
	}
}

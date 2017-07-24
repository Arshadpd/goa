package dsl

import (
	"goa.design/goa.v2/design"
	"goa.design/goa.v2/eval"
)

// Payload defines the data type of an method input. Payload also makes the
// input required.
//
// Payload may appear in a Method expression.
//
// Payload takes one or two arguments. The first argument is either a type or a
// DSL function. If the first argument is a type then an optional DSL may be
// passed as second argument that further specializes the type by providing
// additional validations (e.g. list of required attributes)
//
// Examples:
//
// Method("save"), func() {
//	// Use primitive type.
//	Payload(String)
// }
//
// Method("add", func() {
//     // Define payload data structure inline.
//     Payload(func() {
//         Attribute("left", Int32, "Left operand")
//         Attribute("right", Int32, "Left operand")
//         Required("left", "right")
//     })
// })
//
// Method("add", func() {
//     // Define payload type by reference to user type.
//     Payload(Operands)
// })
//
// Method("divide", func() {
//     // Specify additional required attributes on user type.
//     Payload(Operands, func() {
//         Required("left", "right")
//     })
// })
//
func Payload(val interface{}, fns ...func()) {
	if len(fns) > 1 {
		eval.ReportError("too many arguments")
	}
	e, ok := eval.Current().(*design.MethodExpr)
	if !ok {
		eval.IncompatibleDSL()
		return
	}
	e.Payload = methodDSL("Payload", val, fns...)
}

func methodDSL(suffix string, p interface{}, fns ...func()) *design.AttributeExpr {
	var (
		att *design.AttributeExpr
		fn  func()
	)
	if len(fns) > 0 && fns[0] == nil {
		fns = fns[1:]
	}
	switch actual := p.(type) {
	case func():
		fn = actual
		att = &design.AttributeExpr{Type: &design.Object{}}
	case design.UserType:
		if len(fns) == 0 {
			// Do not duplicate type if it is not customized
			return &design.AttributeExpr{Type: actual}
		}
		att = &design.AttributeExpr{Type: design.Dup(actual)}
	case design.DataType:
		att = &design.AttributeExpr{Type: actual}
	default:
		eval.ReportError("invalid %s argument, must be a type or a function", suffix)
		return nil
	}
	if len(fns) == 1 {
		if fn != nil {
			eval.ReportError("invalid arguments in %s call, must be (type), (func) or (type, func)", suffix)
		}
		fn = fns[0]
	}
	if fn != nil {
		eval.Execute(fn, att)
	}
	return att
}

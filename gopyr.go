// Copyright 2018 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/go-python/gpython/ast"
	"github.com/go-python/gpython/parser"
	"github.com/go-python/gpython/py"
)

var (
	debugLevel   int
	panicUnknown bool
	verbose      bool
)

func unknown(typ string, v interface{}) string {
	msg := fmt.Sprintf("UNKNOWN-%v: %T %#v", typ, v, v)

	if expr, ok := v.(ast.Expr); ok {
		msg += fmt.Sprintf(" at line %d, col %d", expr.GetLineno(), expr.GetColOffset())
	}

	if panicUnknown {
		panic(msg)
	}

	return msg
}

func strBoolOp(op ast.BoolOpNumber) string {
	switch op {
	case ast.And:
		return "&&"

	case ast.Or:
		return "||"
	}

	return unknown("BOOLOP", op.String())
}

func strUnary(op ast.UnaryOpNumber) string {
	switch op {
	case ast.Not:
		return "!"

	case ast.UAdd:
		return "+"

	case ast.USub:
		return "-"
	}

	return unknown("UNARY", op.String())
}

func strOp(op ast.OperatorNumber) string {
	switch op {
	case ast.Add:
		return "+"
	case ast.Sub:
		return "-"
	case ast.Mult:
		return "*"
	case ast.Div:
		return "/"
	case ast.Modulo:
		return "%"
	case ast.Pow:
		return "**"
	case ast.LShift:
		return "<<"
	case ast.RShift:
		return ">>"
	case ast.BitOr:
		return "|"
	case ast.BitXor:
		return "^"
	case ast.BitAnd:
		return "&"
	case ast.FloorDiv:
		return "//"
	}

	return unknown("OP", op.String())
}

func strCmpOp(op ast.CmpOp) string {
	switch op {
	case ast.Eq:
		return "=="
	case ast.NotEq:
		return "!="
	case ast.Lt:
		return "<"
	case ast.LtE:
		return "<="
	case ast.Gt:
		return ">"
	case ast.GtE:
		return ">="
	case ast.Is:
		return "==" // is
	case ast.IsNot:
		return "!=" // is not
	case ast.In:
		return "in"
	case ast.NotIn:
		return "not in"
	}

	return unknown("CMPOP", op.String())
}

func compOps(ops []ast.CmpOp, exprs []ast.Expr) string {
	if len(ops) == 0 {
		return ""
	}

	return fmt.Sprintf(" %v %v", strCmpOp(ops[0]), strExpr(exprs[0])) + compOps(ops[1:], exprs[1:])
}

func strSlice(name ast.Expr, value ast.Slicer) string {
	ret := fmt.Sprintf("%v[", strExpr(name))

	switch s := value.(type) {
	case *ast.Slice:
		if s.Lower != nil {
			ret += strExpr(s.Lower)
		}
		ret += ":"
		if s.Upper != nil {
			ret += strExpr(s.Lower)
		}
		if s.Step != nil {
			ret += ":" + strExpr(s.Lower)
		}

	case *ast.Index:
		ret += strExpr(s.Value)

	case *ast.ExtSlice:
		panic("ExtSlice not implemented")
	}

	return ret + "]"
}

func strExpr(expr interface{}) string {
	if verbose {
		fmt.Printf("XXX %T %#v\n\n", expr, expr)
	}

	switch v := expr.(type) {
	case []ast.Expr:
		var exprs []string
		for _, e := range v {
			exprs = append(exprs, strExpr(e))
		}
		return strings.Join(exprs, ", ")

	case []*ast.Keyword:
		var kwords []string
		for _, k := range v {
			kwords = append(kwords, fmt.Sprintf("%v=%v", strId(k.Arg), strExpr(k.Value)))
		}
		return strings.Join(kwords, ", ")

	case *ast.Tuple:
		var exprs []string
		for _, e := range v.Elts {
			exprs = append(exprs, strExpr(e))
		}
		return "list{" + strings.Join(exprs, ", ") + "}"

	case *ast.List:
		var exprs []string
		for _, e := range v.Elts {
			exprs = append(exprs, strExpr(e))
		}
		return "array{" + strings.Join(exprs, ", ") + "}"

	case *ast.Num:
		s, _ := py.Str(v.N)
		return string(s.(py.String))

	case *ast.NameConstant:
		switch v.Value {
		case py.None:
			return "nil"

		case py.True:
			return "true"

		case py.False:
			return "false"

		default:
			s, _ := py.Str(v.Value)
			return string(s.(py.String))
		}

	case *ast.Str:
		return strconv.Quote(string(v.S))

	case *ast.UnaryOp:
		if v.Op == ast.Invert {
			return fmt.Sprintf("-(%v+1)", strUnary(v.Op), strExpr(v.Operand))
		} else {
			return fmt.Sprintf("%v%v", strUnary(v.Op), strExpr(v.Operand))
		}

	case *ast.BoolOp:
		var bexprs []string
		for _, e := range v.Values {
			bexprs = append(bexprs, strExpr(e))
		}
		return strings.Join(bexprs, " "+strBoolOp(v.Op)+" ")

	case *ast.BinOp:
		return fmt.Sprintf("%v %v %v", strExpr(v.Left), strOp(v.Op), strExpr(v.Right))

	case *ast.Compare:
		return strExpr(v.Left) + compOps(v.Ops, v.Comparators)

	case *ast.Name:
		return string(v.Id)

	case *ast.Attribute:
		return convertName(strExpr(v.Value) + "." + string(v.Attr))

	case *ast.Subscript:
		return strSlice(v.Value, v.Slice)

	case *ast.Call:
		return strCall(v)

	case *ast.Lambda:
		return fmt.Sprintf("func(%v) {  return %s; }", strFunctionArguments(v.Args), strExpr(v.Body))

	case *ast.IfExp:
		return fmt.Sprintf("func() { if %v { return %v } else { return %v }}()",
			strExpr(v.Test), strExpr(v.Body), strExpr(v.Orelse))
	}

	return unknown("EXPR", expr)
}

// an interfact to "print" stuff
type Printable interface {
	Print(args ...interface{})
	Println(args ...interface{})
	Printf(sfmt string, args ...interface{})
}

// Indent implements a Printable, indenting the output
type Indent string

func (indent Indent) Incr() Indent {
	return indent + Indent("  ")
}

func (indent Indent) Print(args ...interface{}) {
	fmt.Print(indent)
	fmt.Print(args...)
}

func (indent Indent) Println(args ...interface{}) {
	fmt.Print(indent)
	fmt.Println(args...)
}

func (indent Indent) Printf(sfmt string, args ...interface{}) {
	fmt.Print(indent)
	fmt.Printf(sfmt, args...)
}

var NoIndent Indent

// PrintableString implements a Printble that writes to a strings (actually strings.Builder)
type PrintableString strings.Builder

func (s PrintableString) Print(args ...interface{}) {
	sb := strings.Builder(s)
	fmt.Fprint(&sb, args...)
}

func (s PrintableString) Println(args ...interface{}) {
	sb := strings.Builder(s)
	fmt.Fprintln(&sb, args...)
}

func (s PrintableString) Printf(sfmt string, args ...interface{}) {
	sb := strings.Builder(s)
	fmt.Fprintf(&sb, sfmt, args...)
}

func convertName(s string) string {
	switch {
	case s == "sys.stdin":
		s = "os.Stdin"

	case s == "sys.stdout":
		s = "os.Stdout"

	case s == "sys.stderr":
		s = "os.Stderr"

	case strings.HasPrefix(s, "sys.stdin."):
		s = "os.Stdin." + s[10:]

	case strings.HasPrefix(s, "sys.stdout."):
		s = "os.Stdout." + s[11:]

	case strings.HasPrefix(s, "sys.stderr."):
		s = "os.Stderr." + s[11:]
	}

	return s
}

func strId(id ast.Identifier) string {
	return convertName(string(id))
}

func strFunctionArguments(args *ast.Arguments) string {
	if args == nil {
		return ""
	}

	var buf strings.Builder

	for _, arg := range args.Args {
		if buf.Len() > 0 {
			buf.WriteString(", ")
		}

		if arg.Annotation != nil {
			buf.WriteString(strExpr(arg.Annotation))
			buf.WriteString(" ")
		}

		buf.WriteString(strId(arg.Arg))
	}

	for i, arg := range args.Kwonlyargs {
		if buf.Len() > 0 {
			buf.WriteString(", ")
		}

		if arg.Annotation != nil {
			buf.WriteString(strExpr(arg.Annotation))
			buf.WriteString(" ")
		}

		buf.WriteString(strId(arg.Arg))
		buf.WriteString("=")
		buf.WriteString(strExpr(args.KwDefaults[i]))
	}

	if args.Vararg != nil {
		if buf.Len() > 0 {
			buf.WriteString(", ")
		}

		buf.WriteString("...")
		buf.WriteString(strId(args.Vararg.Arg)) // annotation ?
	}

	if args.Kwarg != nil {
		if buf.Len() > 0 {
			buf.WriteString(", ")
		}

		buf.WriteString("...")
		buf.WriteString(strId(args.Kwarg.Arg)) // annotation ?
	}

	// XXX: what is arg.Defaults ?

	return buf.String()
}

func strCall(call *ast.Call) string {
	var buf strings.Builder

	cfunc := strExpr(call.Func)

	switch {
	case cfunc == "print":
		cfunc = "fmt.Println" // check for print parameters, could be fmt.Print, fmt.Fprint, etc.

	case strings.HasSuffix(cfunc, ".upper"): // check for arguments
		cfunc = strings.TrimSuffix(cfunc, "upper")
		return fmt.Sprintf("strings.ToUpper(%v)", cfunc)

	case strings.HasSuffix(cfunc, ".lower"): // check for arguments
		cfunc = strings.TrimSuffix(cfunc, "lower")
		return fmt.Sprintf("strings.ToLower(%v)", cfunc)

	case strings.HasSuffix(cfunc, ".startswith"):
		cfunc = fmt.Sprintf("strings.HasPrefix(%v, ", cfunc)

	case strings.HasSuffix(cfunc, ".endswith"):
		cfunc = fmt.Sprintf("strings.HasSuffix(%v, ", cfunc)
	}

	buf.WriteString(cfunc)
	buf.WriteString("(")

	for i, arg := range call.Args {
		if i > 0 {
			buf.WriteString(", ")
		}

		buf.WriteString(strExpr(arg))
	}

	if len(call.Keywords) > 0 {
		if buf.Len() > 0 {
			buf.WriteString(", ")
		}

		buf.WriteString(strExpr(call.Keywords))
	}

	if call.Starargs != nil {
		if buf.Len() > 0 {
			buf.WriteString(", ")
		}

		buf.WriteString("...")
		buf.WriteString(strExpr(call.Starargs))
	}

	if call.Kwargs != nil {
		if buf.Len() > 0 {
			buf.WriteString(", ")
		}

		buf.WriteString("...")
		buf.WriteString(strExpr(call.Kwargs))
	}

	buf.WriteString(")")
	return buf.String()
}

func printBody(indent Indent, body []ast.Stmt, nested bool) {
	for _, stmt := range body {
		if expr, ok := stmt.(*ast.ExprStmt); ok {
			if s, ok := expr.Value.(*ast.Str); ok {
				// a top level string expression is a __doc__ string
				indent.Println("/*")
				indent.Println(s.S)
				indent.Println("*/")
				continue
			}
		}

		ast.Walk(stmt, func(node ast.Ast) bool {
			if verbose {
				fmt.Printf("$$$ %T %#v\n\n", node, node)
			}

			switch v := node.(type) {
			case *ast.Str:
				indent.Printf("%q", v.S)

			case *ast.Num:
				indent.Printf("%v", v.N)

			case *ast.Name:
				indent.Printf("%v", strId(v.Id))

			case *ast.ImportFrom:
				for _, i := range v.Names {
					if i.AsName != "" {
						indent.Printf("import %v \"%v.%v\"\n", i.AsName, v.Module, i.Name)
					} else {
						indent.Printf("import \"%v/%v\"\n", v.Module, i.Name)
					}
				}

			case *ast.Import:
				for _, i := range v.Names {
					if i.AsName != "" {
						indent.Printf("import %s %q\n", i.AsName, i.Name)
					} else {
						indent.Printf("import %q\n", i.Name)
					}
				}

			case *ast.Assign:
				indent.Printf("%v = %v\n", strExpr(v.Targets), strExpr(v.Value))

			case *ast.ExprStmt:
				indent.Println(strExpr(v.Value))

			case *ast.Pass:
				indent.Println("pass")

			case *ast.Return:
				if v.Value == nil {
					indent.Println("return")
				} else {
					indent.Println("return", strExpr(v.Value))
				}

			case *ast.If:
				indentif := indent
				if nested {
					indentif = NoIndent
				}

				indentif.Printf("if %v {\n", strExpr(v.Test))
				printBody(indent.Incr(), v.Body, false)
				for i, e := range v.Orelse {
					if _, ok := e.(*ast.If); ok {
						indent.Printf("} else ")
						printBody(indent, []ast.Stmt{e}, true)
						continue
					}

					indent.Println("} else {")
					printBody(indent.Incr(), v.Orelse[i:], false)
					break
				}

				if !nested {
					indent.Println("}")
				}

			case *ast.For:
				if c, ok := v.Iter.(*ast.Call); ok { // check for "for x in range(n)"
					f := strExpr(c.Func)

					if f == "range" {
						if len(c.Args) < 1 || len(c.Args) > 3 {
							panic(f + " expects 1 to 3 arguments")
						}

						start := "0"
						step := "1"

						var stop string

						if len(c.Args) == 1 {
							stop = strExpr(c.Args[0])
						} else {
							start = strExpr(c.Args[0])
							stop = strExpr(c.Args[1])

							if len(c.Args) > 2 {
								step = strExpr(c.Args[2])
							}
						}

						t := strExpr(v.Target)
						indent.Printf("for %v := %v; %v < %v; %v += %v {\n",
							t, start, t, stop, t, step)
					} else {
						indent.Printf("for %v := range %v {\n",
							strExpr(v.Target), strExpr(v.Iter))
					}
				} else { // for x in iterable
					indent.Printf("for %v := range %v {\n", strExpr(v.Target), strExpr(v.Iter))
				}

				printBody(indent.Incr(), v.Body, false)
				if len(v.Orelse) > 0 {
					indent.Println("} else {")
					printBody(indent.Incr(), v.Orelse, false)
				}
				indent.Println("}")

			case *ast.While:
				indent.Printf("for %v {\n", strExpr(v.Test))
				printBody(indent.Incr(), v.Body, false)
				if len(v.Orelse) > 0 {
					indent.Println("} else {")
					printBody(indent.Incr(), v.Orelse, false)
				}
				indent.Println("}")

			case *ast.FunctionDef:
				indent.Printf("func %v(%v) {\n", v.Name, strFunctionArguments(v.Args))
				printBody(indent.Incr(), v.Body, false)
				indent.Println("}")

			case *ast.ClassDef:
				for _, d := range v.DecoratorList {
					indent.Printf("// @%v\n", strExpr(d))
				}

				indent.Printf("type %v struct {", v.Name)

				if len(v.Bases) > 0 || len(v.Keywords) > 0 {
					fmt.Printf(" //")

					if len(v.Bases) > 0 {
						fmt.Printf(" %v", strExpr(v.Bases))
					}

					if len(v.Keywords) > 0 {
						fmt.Printf(" %v", strExpr(v.Keywords))
					}
				}
				fmt.Println()
				indent.Println("}")

				if len(v.Body) == 1 {
					if _, ok := v.Body[0].(*ast.Pass); ok {
						break
					}
				}

				fmt.Println()
				printBody(indent, v.Body, false)

			case *ast.Arg:
				fmt.Print(v.Arg, v.Annotation)

			default:
				indent.Println(unknown("STMT", node))
				return true
			}

			return false
		})
	}
}

func printPrologue() {
	fmt.Println(`
package converted // converted by ppy

type dict = map[string]interface{}
type list = []interface{}
`)
}

func main() {
	flag.IntVar(&debugLevel, "d", debugLevel, "Debug level 0-4")
	flag.BoolVar(&panicUnknown, "panic", panicUnknown, "panic on unknown expression, to get a stacktrace")
	flag.BoolVar(&verbose, "verbose", verbose, "print statement and expressions")
	flag.Parse()

	parser.SetDebug(debugLevel)

	if len(flag.Args()) == 0 {
		log.Printf("Need files to parse")
		os.Exit(1)
	}
	for _, path := range flag.Args() {
		in, err := os.Open(path)
		if err != nil {
			log.Fatal(err)
		}

		defer in.Close()
		if debugLevel > 0 {
			fmt.Printf(path, "-----------------\n")
		}

		tree, err := parser.Parse(in, path, "exec")
		if err != nil {
			log.Fatal(err)
		}

		m, ok := tree.(*ast.Module)
		if !ok {
			log.Fatal("expected Module, got", tree)
		}

		// where do I get the module name ?
		printPrologue()
		printBody("", m.Body, false)
	}
}

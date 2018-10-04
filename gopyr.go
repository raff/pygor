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

	gokeywords = map[string]string{
		"func": "funcΠ",

		// Convert python names to gopyr names
		"str":   "string",
		"dict":  "Dict",
		"list":  "List",
		"tuple": "Tuple",

		// these are not go keywords but they are used by gopyr
		"Any":   "AnyΠ",
		"Dict":  "DictΠ",
		"List":  "ListΠ",
		"Tuple": "TupleΠ",
	}
)

func rename(s string) string {
	if n, ok := gokeywords[s]; ok {
		return n
	}

	return s
}

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

// Indent implements print functions with indentation
type Indent string

func (indent Indent) Incr() Indent {
	return indent + Indent("  ")
}

func (indent Indent) Decr() Indent {
	return Indent(indent[2:])
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

type ScopeType int

const (
	SModule ScopeType = iota
	SClass
	SMethod
	SFunction
)

type Scope struct {
	Name   string
	Type   ScopeType
	indent Indent

	next *Scope
	prev *Scope
}

func (s *Scope) Incr() {
	s.indent = s.indent.Incr()
}

func (s *Scope) Decr() {
	s.indent = s.indent.Decr()
}

func (s *Scope) strBoolOp(op ast.BoolOpNumber) string {
	switch op {
	case ast.And:
		return "&&"

	case ast.Or:
		return "||"
	}

	return unknown("BOOLOP", op.String())
}

func (s *Scope) strUnary(op ast.UnaryOpNumber) string {
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

func (s *Scope) strOp(op ast.OperatorNumber) string {
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

func (s *Scope) strCmpOp(op ast.CmpOp) string {
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

func (s *Scope) compOps(ops []ast.CmpOp, exprs []ast.Expr) string {
	if len(ops) == 0 {
		return ""
	}

	return fmt.Sprintf(" %v %v", s.strCmpOp(ops[0]), s.strExpr(exprs[0])) + s.compOps(ops[1:], exprs[1:])
}

func (s *Scope) strSlice(name ast.Expr, value ast.Slicer) string {
	ret := fmt.Sprintf("%v[", s.strExpr(name))

	switch sl := value.(type) {
	case *ast.Slice:
		if sl.Lower != nil {
			ret += s.strExpr(sl.Lower)
		}
		ret += ":"
		if sl.Upper != nil {
			ret += s.strExpr(sl.Lower)
		}
		if sl.Step != nil {
			ret += ":" + s.strExpr(sl.Lower)
		}

	case *ast.Index:
		ret += s.strExpr(sl.Value)

	case *ast.ExtSlice:
		panic("ExtSlice not implemented")
	}

	return ret + "]"
}

func (s *Scope) strIdentifiers(l []ast.Identifier) string {
	var ls []string
	for _, i := range l {
		ls = append(ls, strId(i))
	}
	return strings.Join(ls, ", ")
}

func (s *Scope) strExprList(l []ast.Expr, sep string) string {
	var exprs []string
	for _, e := range l {
		exprs = append(exprs, s.strExpr(e))
	}
	return strings.Join(exprs, sep)
}

func (s *Scope) strExpr(expr interface{}) string {
	if verbose {
		fmt.Printf("XXX %T %#v\n\n", expr, expr)
	}

	switch v := expr.(type) {
	case []ast.Expr:
		return s.strExprList(v, ", ")

	case []*ast.Keyword:
		var kwords []string
		for _, k := range v {
			kwords = append(kwords, fmt.Sprintf("%v=%v", strId(k.Arg), s.strExpr(k.Value)))
		}
		return strings.Join(kwords, ", ")

	case *ast.Tuple:
		return "Tuple{" + s.strExprList(v.Elts, ", ") + "}"

	case *ast.List:
		return "List{" + s.strExprList(v.Elts, ", ") + "}"

	case *ast.Dict:
		var kvals []string
		for i, k := range v.Keys {
			kvals = append(kvals, fmt.Sprintf("%v: %v", s.strExpr(k), s.strExpr(v.Values[i])))
		}
		return "Dict{" + strings.Join(kvals, ", ") + "}"

	case *ast.Num:
		s, _ := py.Str(v.N)
		return string(s.(py.String))

	case ast.Identifier:
		return strId(v)

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
			return fmt.Sprintf("-(%v+1)", s.strUnary(v.Op), s.strExpr(v.Operand))
		} else {
			return fmt.Sprintf("%v%v", s.strUnary(v.Op), s.strExpr(v.Operand))
		}

	case *ast.BoolOp:
		var bexprs []string
		for _, e := range v.Values {
			bexprs = append(bexprs, s.strExpr(e))
		}
		return strings.Join(bexprs, " "+s.strBoolOp(v.Op)+" ")

	case *ast.BinOp:
		return fmt.Sprintf("%v %v %v", s.strExpr(v.Left), s.strOp(v.Op), s.strExpr(v.Right))

	case *ast.Compare:
		return s.strExpr(v.Left) + s.compOps(v.Ops, v.Comparators)

	case *ast.Name:
		return strId(v.Id)

	case *ast.Attribute:
		return convertName(s.strExpr(v.Value) + "." + string(v.Attr))

	case *ast.Subscript:
		return s.strSlice(v.Value, v.Slice)

	case *ast.Call:
		return s.strCall(v)

	case *ast.Lambda:
		return fmt.Sprintf("func(%v) {  return %s; }", s.strFunctionArguments(v.Args), s.strExpr(v.Body))

	case *ast.IfExp:
		return fmt.Sprintf("func() { if %v { return %v } else { return %v }}()",
			s.strExpr(v.Test), s.strExpr(v.Body), s.strExpr(v.Orelse))

	case ast.Comprehension:
		target := s.strExpr(v.Target)
		if tuple, ok := v.Target.(*ast.Tuple); ok {
			target = s.strExpr(tuple.Elts)
		}
		if !strings.Contains(target, ", ") { // k,v
			target = "_, " + target
		}
		ret := fmt.Sprintf("for %v := range %v { ", target, s.strExpr(v.Iter))
		close := "}"
		if len(v.Ifs) > 0 {
			ret += fmt.Sprintf("if %v { ", s.strExprList(v.Ifs, " && "))
			close += "}"
		}

		return ret + "@elt@ " + close

	case *ast.ListComp:
		inner := "@elt@"

		for _, g := range v.Generators {
			gen := s.strExpr(g)
			inner = strings.Replace(inner, "@elt@", gen, 1)
		}

		gen := fmt.Sprintf("lc = append(lc, %v)", s.strExpr(v.Elt))
		return "func() (lc List) { " + strings.Replace(inner, "@elt@", gen, 1) + "; return }()"

	case *ast.DictComp:
		inner := "@elt@"

		for _, g := range v.Generators {
			gen := s.strExpr(g)
			inner = strings.Replace(inner, "@elt@", gen, 1)
		}

		gen := fmt.Sprintf("mm[%v] = %v", s.strExpr(v.Key), s.strExpr(v.Value))
		return "func() (mm Dict) { mm = Dict{}; " + strings.Replace(inner, "@elt@", gen, 1) + "; return }()"

	case *ast.GeneratorExp:
		inner := "@elt@"

		for _, g := range v.Generators {
			gen := s.strExpr(g)
			inner = strings.Replace(inner, "@elt@", gen, 1)
		}

		gen := fmt.Sprintf("c <- %v", s.strExpr(v.Elt))
		return "func() (c chan Any) { c = make(chan Any); go func() { " +
			strings.Replace(inner, "@elt@", gen, 1) +
			"; close(c) }(); return }()"
	}

	return unknown("EXPR", expr)
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

	return rename(s)
}

func strId(id ast.Identifier) string {
	return convertName(string(id))
}

func (s *Scope) strFunctionArguments(args *ast.Arguments) string {
	if args == nil {
		return ""
	}

	var buf strings.Builder

	for _, arg := range args.Args {
		if buf.Len() > 0 {
			buf.WriteString(", ")
		}

		if arg.Annotation != nil {
			buf.WriteString(s.strExpr(arg.Annotation))
			buf.WriteString(" ")
		}

		buf.WriteString(strId(arg.Arg))
		buf.WriteString(" Any") // can't guess argument types
	}

	for i, arg := range args.Kwonlyargs {
		if buf.Len() > 0 {
			buf.WriteString(", ")
		}

		if arg.Annotation != nil {
			buf.WriteString(s.strExpr(arg.Annotation))
			buf.WriteString(" ")
		}

		buf.WriteString(strId(arg.Arg))
		buf.WriteString(" Any = ") // here I could guess based on the default values
		buf.WriteString("=")
		buf.WriteString(s.strExpr(args.KwDefaults[i]))
	}

	if args.Vararg != nil {
		if buf.Len() > 0 {
			buf.WriteString(", ")
		}

		buf.WriteString(strId(args.Vararg.Arg)) // annotation ?
		buf.WriteString(" ...Any")
	}

	if args.Kwarg != nil {
		if buf.Len() > 0 {
			buf.WriteString(", ")
		}

		buf.WriteString(strId(args.Kwarg.Arg)) // annotation ?
		buf.WriteString(" ...Any")
	}

	// XXX: what is arg.Defaults ?

	return buf.String()
}

func (s *Scope) strCall(call *ast.Call) string {
	var buf strings.Builder

	cfunc := s.strExpr(call.Func)

	switch {
	case cfunc == "print":
		cfunc = "fmt.Println" // check for print parameters, could be fmt.Print, fmt.Fprint, etc.

	case cfunc == "open":
		cfunc = "os.Open"

	case strings.HasSuffix(cfunc, ".read"):
		cfunc = strings.TrimSuffix(cfunc, ".read") + ".Read"

	case strings.HasSuffix(cfunc, ".write"):
		cfunc = strings.TrimSuffix(cfunc, ".write") + ".Write"

	case strings.HasSuffix(cfunc, ".close"):
		cfunc = strings.TrimSuffix(cfunc, ".close") + ".Close"

	case strings.HasSuffix(cfunc, ".items"):
		return strings.TrimSuffix(cfunc, ".items")

	case strings.HasSuffix(cfunc, ".upper"): // check for arguments
		cfunc = strings.TrimSuffix(cfunc, ".upper")
		return fmt.Sprintf("strings.ToUpper(%v)", cfunc)

	case strings.HasSuffix(cfunc, ".lower"): // check for arguments
		cfunc = strings.TrimSuffix(cfunc, ".lower")
		return fmt.Sprintf("strings.ToLower(%v)", cfunc)

	case strings.HasSuffix(cfunc, ".startswith"):
		cfunc = fmt.Sprintf("strings.HasPrefix(%v, ", cfunc)

	case strings.HasSuffix(cfunc, ".endswith"):
		cfunc = fmt.Sprintf("strings.HasSuffix(%v, ", cfunc)

	case cfunc == "isinstance": // isinstance(obj, type)
		if len(call.Args) == 2 {
			obj := s.strExpr(call.Args[0])
			otype := s.strExpr(call.Args[1])
			return fmt.Sprintf("func() bool { _, ok := %v.(%v); return ok }()", obj, otype)
		}
	}

	buf.WriteString(cfunc)
	buf.WriteString("(")

	args := false

	for _, arg := range call.Args {
		if args {
			buf.WriteString(", ")
		} else {
			args = true
		}

		buf.WriteString(s.strExpr(arg))
	}

	if len(call.Keywords) > 0 {
		if args {
			buf.WriteString(", ")
		} else {
			args = true
		}

		buf.WriteString(s.strExpr(call.Keywords))
	}

	if call.Starargs != nil {
		if args {
			buf.WriteString(", ")
		} else {
			args = true
		}

		buf.WriteString("...")
		buf.WriteString(s.strExpr(call.Starargs))
	}

	if call.Kwargs != nil {
		if args {
			buf.WriteString(", ")
		} else {
			args = true
		}

		buf.WriteString("...")
		buf.WriteString(s.strExpr(call.Kwargs))
	}

	buf.WriteString(")")
	return buf.String()
}

func (s *Scope) printBody(body []ast.Stmt, nested bool) {
	if !nested {
		s.Incr()
		defer s.Decr()
	}

	for _, stmt := range body {
		if expr, ok := stmt.(*ast.ExprStmt); ok {
			if str, ok := expr.Value.(*ast.Str); ok {
				// a top level string expression is a __doc__ string
				s.indent.Println("/*")
				s.indent.Println(str.S)
				s.indent.Println("*/")
				continue
			}
		}

		ast.Walk(stmt, func(node ast.Ast) bool {
			if verbose {
				fmt.Printf("$$$ %T %#v\n\n", node, node)
			}

			switch v := node.(type) {
			case *ast.Str:
				s.indent.Printf("%q", v.S)

			case *ast.Num:
				s.indent.Printf("%v", v.N)

			case *ast.Name:
				s.indent.Printf("%v", strId(v.Id))

			case *ast.Arg:
				fmt.Print(v.Arg, v.Annotation)

			case *ast.ImportFrom:
				for _, i := range v.Names {
					if i.AsName != "" {
						s.indent.Printf("import %v \"%v.%v\"\n", i.AsName, v.Module, i.Name)
					} else {
						s.indent.Printf("import \"%v/%v\"\n", v.Module, i.Name)
					}
				}

			case *ast.Import:
				for _, i := range v.Names {
					if i.AsName != "" {
						s.indent.Printf("import %s %q\n", i.AsName, i.Name)
					} else {
						s.indent.Printf("import %q\n", i.Name)
					}
				}

			case *ast.Assign:
				s.indent.Printf("%v = %v\n", s.strExpr(v.Targets), s.strExpr(v.Value))

			case *ast.AugAssign:
				s.indent.Printf("%v %v= %v\n", s.strExpr(v.Target), s.strOp(v.Op), s.strExpr(v.Value))

			case *ast.ExprStmt:
				s.indent.Println(s.strExpr(v.Value))

			case *ast.Pass:
				s.indent.Println("// pass")

			case *ast.Return:
				if v.Value == nil {
					s.indent.Println("return")
				} else {
					s.indent.Println("return", s.strExpr(v.Value))
				}

			case *ast.If:
				indentif := s.indent
				if nested {
					indentif = NoIndent
				}

				indentif.Printf("if %v {\n", s.strExpr(v.Test))
				s.printBody(v.Body, false)
				for i, e := range v.Orelse {
					if _, ok := e.(*ast.If); ok {
						s.indent.Printf("} else ")
						s.printBody([]ast.Stmt{e}, true)
						continue
					}

					s.indent.Println("} else {")
					s.printBody(v.Orelse[i:], false)
					break
				}

				if !nested {
					s.indent.Println("}")
				}

			case *ast.For:
				if c, ok := v.Iter.(*ast.Call); ok { // check for "for x in range(n)"
					f := s.strExpr(c.Func)

					if f == "range" {
						if len(c.Args) < 1 || len(c.Args) > 3 {
							panic(f + " expects 1 to 3 arguments")
						}

						start := "0"
						step := "1"

						var stop string

						if len(c.Args) == 1 {
							stop = s.strExpr(c.Args[0])
						} else {
							start = s.strExpr(c.Args[0])
							stop = s.strExpr(c.Args[1])

							if len(c.Args) > 2 {
								step = s.strExpr(c.Args[2])
							}
						}

						t := s.strExpr(v.Target)
						s.indent.Printf("for %v := %v; %v < %v; %v += %v {\n",
							t, start, t, stop, t, step)
					} else {
						t := s.strExpr(v.Target)
						if tuple, ok := v.Target.(*ast.Tuple); ok {
							t = s.strExpr(tuple.Elts)
						}
						s.indent.Printf("for %v := range %v {\n", t, s.strExpr(v.Iter))
					}
				} else { // for x in iterable
					s.indent.Printf("for %v := range %v {\n", s.strExpr(v.Target), s.strExpr(v.Iter))
				}

				s.printBody(v.Body, false)
				if len(v.Orelse) > 0 {
					s.indent.Println("} else {")
					s.printBody(v.Orelse, false)
				}
				s.indent.Println("}")

			case *ast.While:
				s.indent.Printf("for %v {\n", s.strExpr(v.Test))
				s.printBody(v.Body, false)
				if len(v.Orelse) > 0 {
					s.indent.Println("} else {")
					s.printBody(v.Orelse, false)
				}
				s.indent.Println("}")

			case *ast.FunctionDef:
				s.indent.Printf("func %v(%v) {\n", s.strExpr(v.Name), s.strFunctionArguments(v.Args))
				s.printBody(v.Body, false)
				s.indent.Println("}")

			case *ast.ClassDef:
				for _, d := range v.DecoratorList {
					s.indent.Printf("// @%v\n", s.strExpr(d))
				}

				s.indent.Printf("type %v struct {", v.Name)

				if len(v.Bases) > 0 || len(v.Keywords) > 0 {
					fmt.Printf(" //")

					if len(v.Bases) > 0 {
						fmt.Printf(" %v", s.strExpr(v.Bases))
					}

					if len(v.Keywords) > 0 {
						fmt.Printf(" %v", s.strExpr(v.Keywords))
					}
				}
				fmt.Println()
				s.indent.Println("}")

				if len(v.Body) == 1 {
					if _, ok := v.Body[0].(*ast.Pass); ok {
						break
					}
				}

				fmt.Println()
				s.printBody(v.Body, false)

			case *ast.Try:
				s.indent.Println("if err := func() PyException { // try")
				s.printBody(v.Body, false)
				s.indent.Println("}(); err != nil {")

				if len(v.Handlers) > 0 {
					s.Incr()
					s.indent.Println("switch err { // except")

					for _, h := range v.Handlers {
						s.indent.Printf("case %v:", s.strExpr(h.ExprType))
						if h.Name != "" {
							fmt.Printf(" // as %v", h.Name)
						}
						fmt.Println()
						s.printBody(h.Body, false)
					}

					s.indent.Println("}")
					s.Decr()
				}

				if len(v.Orelse) > 0 {
					s.indent.Println("} else {")
					s.printBody(v.Orelse, false)
				}

				if len(v.Finalbody) > 0 {
					s.indent.Println("}; { // finally")
					s.printBody(v.Finalbody, false)
				}
				s.indent.Println("}")

			case *ast.Raise:
				s.indent.Printf("return RaisedException(%v) // raise", s.strExpr(v.Exc))
				if v.Cause != nil {
					fmt.Printf(" cause: %v", s.strExpr(v.Cause))
				}
				fmt.Println()

			case *ast.Assert:
				if v.Msg != nil {
					s.indent.Printf("Assert(%v, %v)\n", s.strExpr(v.Test), s.strExpr(v.Msg))
				} else {
					s.indent.Printf("Assert(%v, %q)\n", s.strExpr(v.Test), s.strExpr(v.Test))
				}

			case *ast.Global:
				s.indent.Println("// global", s.strIdentifiers(v.Names))

			case *ast.Delete:
				for _, t := range v.Targets {
					st := t.(*ast.Subscript)
					if i, ok := st.Slice.(*ast.Index); ok {
						s.indent.Printf("delete(%v, %v)\n", s.strExpr(st.Value), s.strExpr(i.Value))
						continue
					}

					s.indent.Printf("delete %v // %#v\n", s.strExpr(st), st)
				}

			default:
				s.indent.Println(unknown("STMT", node))
				return true
			}

			return false
		})
	}
}

func (s *Scope) printPrologue() {
	fmt.Println(`// converted by gopyr
package converted

import "fmt"
import . "github.com/raff/gopyr/runtime"

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

	var scope Scope

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
		scope.printPrologue()
		scope.printBody(m.Body, false)
	}
}

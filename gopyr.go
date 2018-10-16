package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"

	"github.com/go-python/gpython/ast"
	"github.com/go-python/gpython/parser"
	"github.com/go-python/gpython/py"

	"github.com/raff/jennifer/jen"
)

var (
	debugLevel   int
	panicUnknown bool
	verbose      bool
	mainpackage  bool

	gokeywords = map[string]string{
		"func": "funcΠ",

		// Convert python names to gopyr names
		"str":     "string",
		"float":   "float64",
		"complex": "complex128",
		"dict":    "Dict",
		"list":    "List",
		"tuple":   "Tuple",

		// these are not go keywords but they are used by gopyr
		"Any":   "AnyΠ",
		"Dict":  "DictΠ",
		"List":  "ListΠ",
		"Tuple": "TupleΠ",
	}

	goList      = jen.Qual("github.com/raff/gopyr/runtime", "List")
	goTuple     = jen.Qual("github.com/raff/gopyr/runtime", "Tuple")
	goDict      = jen.Qual("github.com/raff/gopyr/runtime", "Dict")
	goException = jen.Qual("github.com/raff/gopyr/runtime", "PyException")
	goContains  = jen.Qual("github.com/raff/gopyr/runtime", "Contains")
)

func rename(s string) string {
	if n, ok := gokeywords[s]; ok {
		return n
	}

	return s
}

func renameId(id ast.Identifier) string {
	return rename(string(id))
}

func unknown(typ string, v interface{}) *jen.Statement {
	msg := fmt.Sprintf("UNKNOWN-%v: %T %#v", typ, v, v)

	if expr, ok := v.(ast.Expr); ok {
		msg += fmt.Sprintf(" at line %d, col %d", expr.GetLineno(), expr.GetColOffset())
	}

	if panicUnknown {
		panic(msg)
	}

	return jen.Lit(msg)
}

type Scope struct {
	level   int // nesting level
	vars    map[string]struct{}
	imports map[string]string

	next *Scope
	prev *Scope
}

func NewScope() *Scope {
	return &Scope{vars: make(map[string]struct{}), imports: make(map[string]string)}
}

func (s *Scope) Push() *Scope {
	next := &Scope{vars: make(map[string]struct{}), imports: s.imports}
	s.next = next
	next.level = s.level + 1
	next.prev = s
	return next
}

func (s *Scope) Pop() *Scope {
	prev := s.prev
	prev.next = nil
	return prev
}

// check if the element in the expression list are new names
// (and add them to the list of known names)
func (s *Scope) newNames(lexpr []ast.Expr) (ret bool) {
	for _, x := range lexpr {
		var nn string

		switch t := x.(type) {
		case *ast.Name:
			nn = string(t.Id)

		default:
			continue
		}

		// if we have seen the name before, in any scope,
		// it's defined. Otherwise "define" it in the current scope.
		// (but if forceNew is set, these are new names in the scope)
		found := false

		for curr := s; curr != nil; curr = curr.prev {
			if _, ok := curr.vars[nn]; ok {
				found = true
				break
			}
		}

		if !found {
			s.vars[nn] = struct{}{}
			ret = true
		}
	}

	return
}

func (s *Scope) addName(id ast.Identifier) {
	s.vars[string(id)] = struct{}{}
}

func (s *Scope) goBoolOp(op ast.BoolOpNumber) *jen.Statement {
	switch op {
	case ast.And:
		return jen.Op("&&")

	case ast.Or:
		return jen.Op("||")
	}

	return unknown("BOOLOP", op.String())
}

func (s *Scope) goUnary(op ast.UnaryOpNumber) *jen.Statement {
	switch op {
	case ast.Not:
		return jen.Op("!")

	case ast.UAdd:
		return jen.Op("+")

	case ast.USub:
		return jen.Op("-")
	}

	return unknown("UNARY", op.String())
}

func (s *Scope) goOp(op ast.OperatorNumber) *jen.Statement {
	return s.goOpExt(op, "")
}

func (s *Scope) goOpExt(op ast.OperatorNumber, ext string) *jen.Statement {
	switch op {
	case ast.Add:
		return jen.Op("+" + ext)
	case ast.Sub:
		return jen.Op("-" + ext)
	case ast.Mult:
		return jen.Op("*" + ext)
	case ast.Div:
		return jen.Op("/" + ext)
	case ast.Modulo:
		return jen.Op("%" + ext)
	case ast.Pow:
		return jen.Op("**" + ext)
	case ast.LShift:
		return jen.Op("<<" + ext)
	case ast.RShift:
		return jen.Op(">>" + ext)
	case ast.BitOr:
		return jen.Op("|" + ext)
	case ast.BitXor:
		return jen.Op("^" + ext)
	case ast.BitAnd:
		return jen.Op("&" + ext)
	case ast.FloorDiv:
		return jen.Op("//" + ext)
	}

	return unknown("OP", op.String()+ext)
}

func (s *Scope) goCmpOp(op ast.CmpOp) *jen.Statement {
	switch op {
	case ast.Eq:
		return jen.Op("==")
	case ast.NotEq:
		return jen.Op("!=")
	case ast.Lt:
		return jen.Op("<")
	case ast.LtE:
		return jen.Op("<=")
	case ast.Gt:
		return jen.Op(">")
	case ast.GtE:
		return jen.Op(">=")
	case ast.Is:
		return jen.Op("==") // is
	case ast.IsNot:
		return jen.Op("!=") // is not
	case ast.In:
		return jen.Op("in")
	case ast.NotIn:
		return jen.Op("not in")
	}

	return unknown("CMPOP", op.String())
}

func (s *Scope) goSlice(name ast.Expr, value ast.Slicer) *jen.Statement {
	stmt := s.goExpr(name)
	start := jen.Empty()
	end := jen.Empty()

	switch sl := value.(type) {
	case *ast.Slice:
		if sl.Lower != nil {
			start = s.goExpr(sl.Lower)
		}
		if sl.Upper != nil {
			end = s.goExpr(sl.Upper)
		}
		if sl.Step != nil {
			panic("step index not implemented")
		}
		stmt.Add(jen.Index(start, end))

	case *ast.Index:
		stmt.Add(jen.Index(s.goExpr(sl.Value)))

	case *ast.ExtSlice:
		panic("ExtSlice not implemented")
	}

	return stmt
}

func (s *Scope) goIdentifiers(l []ast.Identifier) *jen.Statement {
	return jen.ListFunc(func(g *jen.Group) {
		for _, i := range l {
			g.Add(goId(i))
		}
	})
}

func (s *Scope) goInitialized(otype *jen.Statement, values []ast.Expr) *jen.Statement {
	return jen.Parens(otype.Clone().ValuesFunc(func(g *jen.Group) {
		for _, v := range values {
			g.Add(s.goExpr(v))
		}
	}))
}

func (s *Scope) goExprList(values []ast.Expr) *jen.Statement {
	return jen.ListFunc(func(g *jen.Group) {
		for _, v := range values {
			g.Add(s.goExpr(v))
		}
	})
}

func (s *Scope) goExprOrList(expr ast.Expr) *jen.Statement {
	if tuple, ok := expr.(*ast.Tuple); ok {
		return s.goExpr(tuple.Elts)
	}
	return s.goExpr(expr)
}

func isNone(expr ast.Expr) bool {
	if c, ok := expr.(*ast.NameConstant); ok {
		return c.Value == py.None
	}

	return false
}

func (s *Scope) gomprehension(c ast.Comprehension) (*jen.Statement, *jen.Statement) {
	iter := s.goFor(c.Target, c.Iter)
	cond := iter
	if len(c.Ifs) > 0 {
		ccond := s.goExpr(c.Ifs[0])
		for _, c := range c.Ifs[1:] {
			ccond.Add(jen.Op("&&"))
			ccond.Add(s.goExpr(c))
		}
		cond = jen.If(ccond)
		iter.Block(cond)
	}

	return iter, cond
}

// print k=v either for function definitions (def=true) or for function call (def=false)
func (s *Scope) goKvals(kk []*ast.Keyword, def bool) *jen.Statement {
	return jen.ListFunc(func(g *jen.Group) {
		if def {
			for _, k := range kk {
				g.Add(goId(k.Arg).Commentf("/*=%v*/", s.goExpr(k.Value).GoString()))
			}
		} else {
			for _, k := range kk {
				g.Commentf("/*%v=*/", string(k.Arg)).Add(s.goExpr(k.Value))
			}
		}
	})
}

func (s *Scope) goExpr(expr interface{}) *jen.Statement {
	if verbose {
		fmt.Printf("XXX %T %#v\n\n", expr, expr)
	}

	switch v := expr.(type) {
	case []ast.Expr:
		return s.goExprList(v)

	case []*ast.Keyword:
		return jen.ListFunc(func(g *jen.Group) {
			for _, k := range v {
				g.Add(goId(k.Arg).Commentf("/*=%v*/", s.goExpr(k.Value).GoString()))
			}
		})

	case *ast.Tuple:
		return s.goInitialized(goTuple, v.Elts)

	case *ast.List:
		return s.goInitialized(goList, v.Elts)

	case *ast.Dict:
		return jen.Parens(goDict.Clone().Values(jen.DictFunc(func(d jen.Dict) {
			for i, k := range v.Keys {
				d[s.goExpr(k)] = s.goExpr(v.Values[i])
			}
		})))

	case *ast.Num:
		switch n := v.N.(type) {
		case py.Int:
			return jen.Lit(int(n))

		case py.Float:
			return jen.Lit(float64(n))

		case py.Complex:
			return jen.Lit(complex128(n))

		default:
			panic("invalid number")
		}

	case ast.Identifier:
		return goId(v)

	case *ast.NameConstant:
		switch v.Value {
		case py.None:
			return jen.Nil()

		case py.True:
			return jen.True()

		case py.False:
			return jen.False()

		default:
			s, _ := py.Str(v.Value)
			return jen.Lit(string(s.(py.String)))
		}

	case *ast.Str:
		return jen.Lit(string(v.S))

	case *ast.UnaryOp:
		if v.Op == ast.Invert {
			return s.goUnary(v.Op).Parens(s.goExpr(v.Operand).Op("+").Lit(1))
		} else {
			return s.goUnary(v.Op).Add(s.goExpr(v.Operand))
		}

	case *ast.BoolOp:
		stmt := s.goExpr(v.Values[0])
		for _, x := range v.Values[1:] {
			stmt.Add(s.goBoolOp(v.Op))
			stmt.Add(s.goExpr(x))
		}
		return stmt

	case *ast.BinOp:
		if v.Op == ast.Modulo { // %
			if _, ok := v.Left.(*ast.Str); ok { // this is really a formatting operation
				printfunc := jen.Qual("fmt", "Sprintf")
				printfmt := s.goExpr(v.Left)
				params := s.goExpr(v.Right)
				if tuple, ok := v.Right.(*ast.Tuple); ok {
					params = s.goExprList(tuple.Elts)
				}
				return printfunc.Params(printfmt, params)
			}
		}

		return s.goExpr(v.Left).Add(s.goOp(v.Op)).Add(s.goExpr(v.Right))

	case *ast.Compare:
		stmt := jen.Null()

		left := s.goExpr(v.Left)
		right := (*jen.Statement)(nil)

		for i, op := range v.Ops {
			if right != nil {
				stmt.Op("&&")
				left = right.Clone()
			}

			right = s.goExpr(v.Comparators[i])

			if op == ast.In {
				stmt.Add(goContains.Clone().Call(right, left))
			} else if op == ast.NotIn {
				stmt.Op("!").Add(goContains.Clone().Call(right, left))
			} else {
				stmt.Add(left)
				stmt.Add(s.goCmpOp(op))
				stmt.Add(right)
			}
		}

		return stmt

	case *ast.Name:
		return goId(v.Id)

	case *ast.Attribute:
		if n, ok := v.Value.(*ast.Name); ok && s.imports[string(n.Id)] != "" {
			return jen.Qual(s.imports[string(n.Id)], string(v.Attr))
		}
		return s.goExpr(v.Value).Dot(renameId(v.Attr))

	case *ast.Subscript:
		return s.goSlice(v.Value, v.Slice)

	case *ast.Call:
		return s.goCall(v)

	case *ast.Lambda:
		args, _ := s.goFunctionArguments(v.Args, false)
		return jen.Func().Params(args).Block(s.goExpr(v.Body)).Call()

	case *ast.IfExp:
		return jen.Func().Params().Block(
			jen.If(s.goExpr(v.Test)).
				Block(jen.Return(s.goExpr(v.Body))).
				Else().
				Block(jen.Return(s.goExpr(v.Orelse)))).Call()

	case *ast.ListComp:
		outer, inner := s.gomprehension(v.Generators[0])
		for _, g := range v.Generators[1:] {
			outer1, inner1 := s.gomprehension(g)
			inner.Add(jen.Block(outer1))
			inner = inner1
		}
		inner.Add(jen.Block(jen.Id("lc").Op("=").Append(jen.Id("lc"), s.goExpr(v.Elt))))
		return jen.Func().Params().Params(jen.Id("lc").Add(goList)).Block(outer, jen.Return(jen.Id("lc"))).Call()

	case *ast.DictComp:
		outer, inner := s.gomprehension(v.Generators[0])
		for _, g := range v.Generators[1:] {
			outer1, inner1 := s.gomprehension(g)
			inner.Add(jen.Block(outer1))
			inner = inner1
		}
		inner.Add(jen.Block(jen.Id("mm").Index(s.goExpr(v.Key)).Op("=").Add(s.goExpr(v.Value))))
		return jen.Func().Params().Params(jen.Id("mm").Add(goDict)).Block(
			jen.Id("mm").Op("=").Add(goDict).Values(),
			outer,
			jen.Return()).Call()

	case *ast.GeneratorExp:
		outer, inner := s.gomprehension(v.Generators[0])
		for _, g := range v.Generators[1:] {
			outer1, inner1 := s.gomprehension(g)
			inner.Add(jen.Block(outer1))
			inner = inner1
		}
		inner.Add(jen.Block(jen.Id("c").Op("<-").Add(s.goExpr(v.Elt))))
		return jen.Func().Params().Params(jen.Id("c").Chan().Id("Any")).Block(
			jen.Id("c").Op("=").Make(jen.Chan().Id("Any")),
			jen.Go().Func().Params().Block(outer, jen.Close(jen.Id("c"))).Call(),
			jen.Return(),
		).Call()
	}

	return unknown("EXPR", expr)
}

func goId(id ast.Identifier) *jen.Statement {
	s := string(id)

	switch {
	case s == "sys.stdin":
		return jen.Qual("os", "Stdin")

	case s == "sys.stdout":
		return jen.Qual("os", "Stdout")

	case s == "sys.stderr":
		return jen.Qual("os", "Stderr")

	case strings.HasPrefix(s, "sys.stdin."):
		return jen.Qual("os", "Stdin").Id(s[10:])

	case strings.HasPrefix(s, "sys.stdout."):
		return jen.Qual("os", "Stdout").Id(s[11:])

	case strings.HasPrefix(s, "sys.stderr."):
		return jen.Qual("os", "Stderr").Id(s[11:])
	}

	return jen.Id(rename(s))
}

func (s *Scope) goFunctionArguments(args *ast.Arguments, skipReceiver bool) (*jen.Statement, *ast.Arg) {
	var recv *ast.Arg

	if args == nil {
		return jen.Null(), recv
	}

	var params []jen.Code

	aargs := args.Args
	if skipReceiver && len(aargs) > 0 {
		recv, aargs = aargs[0], aargs[1:]
	}

	for _, arg := range aargs {
		s.addName(arg.Arg)

		p := goId(arg.Arg)
		if arg.Annotation != nil {
			p.Add(s.goExpr(arg.Annotation))
		} else {
			p.Add(jen.Id("Any"))
		}

		params = append(params, p)
	}

	for i, arg := range args.Kwonlyargs {
		s.addName(arg.Arg)

		p := goId(arg.Arg)
		if arg.Annotation != nil {
			p.Add(s.goExpr(arg.Annotation))
		} else {
			p.Add(jen.Id("Any"))
		}

		p.Commentf("/*=%v*/", s.goExpr(args.KwDefaults[i]).GoString())
		params = append(params, p)
	}

	if args.Vararg != nil {
		s.addName(args.Vararg.Arg)

		p := goId(args.Vararg.Arg).Comment("/*...*/")
		if args.Vararg.Annotation != nil {
			p.Add(s.goExpr(args.Vararg.Annotation))
		} else {
			p.Add(jen.Id("Any"))
		}

		params = append(params, p)
	}

	if args.Kwarg != nil {
		s.addName(args.Kwarg.Arg)

		p := goId(args.Kwarg.Arg).Comment("/*...*/")
		if args.Vararg.Annotation != nil {
			p.Add(s.goExpr(args.Kwarg.Annotation))
		} else {
			p.Add(jen.Id("Any"))
		}

		params = append(params, p)
	}

	// XXX: what is arg.Defaults ?

	return jen.List(params...), recv
}

func (s *Scope) goCallParams(params ...ast.Expr) *jen.Statement {
	return jen.ParamsFunc(func(g *jen.Group) {
		for _, p := range params {
			g.Add(s.goExpr(p))
		}
	})
}

func (s *Scope) goCall(call *ast.Call) *jen.Statement {
	cfunc := s.goExpr(call.Func)

	switch ff := call.Func.(type) {
	case *ast.Name:
		switch string(ff.Id) {
		case "print":
			cfunc = jen.Qual("fmt", "Println") // check for print parameters, could be fmt.Print, fmt.Fprint, etc.

		case "open":
			cfunc = jen.Qual("os", "Open") // could also be os.OpenFile

		case "isinstance": // isinstance(obj, type)
			if len(call.Args) == 2 {
				obj := s.goExpr(call.Args[0])
				otype := s.goExpr(call.Args[1])
				comment := jen.Commentf("isinstance(%v, %v)", obj.GoString(), otype.GoString())
				if attr, ok := call.Args[1].(*ast.Attribute); ok {
					otype = jen.Commentf("/*%v*/", s.goExpr(attr.Value).GoString()).Add(s.goExpr(attr.Attr))
				}
				return jen.Func().Params().Bool().Block(
					comment,
					jen.List(jen.Op("_"), jen.Id("ok")).Op(":=").Add(obj).Assert(otype),
					jen.Return(jen.Id("ok")),
				).Call()
			}

		case "type":
			cfunc = jen.Qual("reflect", "Type")
		}

	case *ast.Attribute:
		switch string(ff.Attr) {
		case "read":
			cfunc = s.goExpr(ff.Value).Dot("Read")

		case "write":
			cfunc = s.goExpr(ff.Value).Dot("Write")

		case "close":
			cfunc = s.goExpr(ff.Value).Dot("Close")

		case "items": // as in `for k, v in dict(a=1).items()`
			return s.goExpr(ff.Value) // remove items

		case "upper":
			return jen.Qual("strings", "ToUpper").Call(s.goExpr(ff.Value))

		case "lower":
			return jen.Qual("strings", "ToLower").Call(s.goExpr(ff.Value))

		case "startswith":
			if len(call.Args) == 1 {
				return jen.Qual("strings", "HasPrefix").Call(s.goExpr(ff.Value), s.goExpr(call.Args[0]))
			}

		case "endswith":
			if len(call.Args) == 1 {
				return jen.Qual("strings", "HasSuffix").Call(s.goExpr(ff.Value), s.goExpr(call.Args[0]))
			}

		case "strip":
			if len(call.Args) == 0 {
				return jen.Qual("strings", "TrimSpace").Call(s.goExpr(ff.Value))
			}

		case "split":
			if len(call.Args) == 1 {
				return jen.Qual("strings", "Split").Call(s.goExpr(ff.Value), s.goExpr(call.Args[0]))
			}

		case "join":
			if len(call.Args) == 1 {
				return jen.Qual("strings", "Join").Call(s.goExpr(call.Args[0]), s.goExpr(ff.Value))
			}
		}

		if name, ok := ff.Value.(*ast.Name); ok {
			switch {
			case string(name.Id) == "sys" && string(ff.Attr) == "exit":
				ret := jen.Lit(-1)
				if len(call.Args) > 0 {
					ret = s.goExpr(call.Args[0])
				}
				return jen.Qual("os", "Exit").Call(ret)

			case string(name.Id) == "time" && string(ff.Attr) == "sleep" && len(call.Args) == 1:
				tt := jen.Qual("time", "Duration").Parens(
					s.goExpr(call.Args[0]).Op("*").Float64().Parens(jen.Qual("time", "Second")))
				return jen.Qual("time", "Sleep").Call(tt)

			case string(name.Id) == "time" && string(ff.Attr) == "time" && len(call.Args) == 0:
				return jen.Qual("time", "Now").Call()
			}
		}
	}

	var args []jen.Code

	for _, arg := range call.Args {
		args = append(args, s.goExpr(arg))
	}

	if len(call.Keywords) > 0 {
		args = append(args, s.goKvals(call.Keywords, false))
	}

	if call.Starargs != nil {
		args = append(args, s.goExpr(call.Starargs).Comment("/*...*/"))
	}

	if call.Kwargs != nil {
		args = append(args, s.goExpr(call.Kwargs).Comment("/*...*/"))
	}

	return cfunc.Call(args...)
}

func (s *Scope) goFor(target, iter ast.Expr) *jen.Statement {
	if c, ok := iter.(*ast.Call); ok { // check for "for x in range(n)"
		if n, ok := c.Func.(*ast.Name); ok && string(n.Id) == "range" {
			if len(c.Args) < 1 || len(c.Args) > 3 {
				panic("range expects 1 to 3 arguments")
			}

			start := jen.Lit(0)
			step := jen.Lit(1)

			var stop jen.Code

			if len(c.Args) == 1 {
				stop = s.goExpr(c.Args[0])
			} else {
				start = s.goExpr(c.Args[0])
				stop = s.goExpr(c.Args[1])

				if len(c.Args) > 2 {
					step = s.goExpr(c.Args[2])
				}
			}

			t := s.goExpr(target)

			return jen.For(t.Clone().Op(":=").Add(start),
				t.Clone().Op("<").Add(stop),
				t.Clone().Op("+=").Add(step))
		}

		t := s.goExprOrList(target)
		return jen.For(t.Op(":=").Range().Add(s.goExpr(iter)))
	}

	// for x in iterable
	return jen.For(s.goExpr(target).Op(":=").Range().Add(s.goExpr(iter)))
}

// parse a block/list of statements
func (s *Scope) parseBody(classname string, body []ast.Stmt) *jen.Statement {
	p, _, _ := s.parseBodyEx(classname, body)
	return p
}

// parse a block/list of statements anre returns
// - the block, as single statement
// - the list of statements (useful only in the main module)
// - the type of return (in a function body, true: yield, false: return)
func (s *Scope) parseBodyEx(classname string, body []ast.Stmt) (*jen.Statement, []*jen.Statement, bool) {
	s.level += 1

	parsed := jen.Null()
	stmts := []*jen.Statement{}
	generator := false

	add := func(s *jen.Statement) {
		if verbose {
			fmt.Println("GGG", s.GoString())
		}

		parsed.Add(s)
		stmts = append(stmts, s)
	}

	for i, stmt := range body {
		if i > 0 {
			add(jen.Line())
		}

		if expr, ok := stmt.(*ast.ExprStmt); ok {
			if str, ok := expr.Value.(*ast.Str); ok {
				// a top level string expression is a __doc__ string
				add(jen.Comment(string(str.S)))
				continue
			}
		}

		switch v := stmt.(type) {
		case *ast.ImportFrom:
			s.imports[string(v.Module)] = string(v.Module)
			for _, i := range v.Names {
				if i.AsName != "" {
					add(jen.Commentf("import %v %q // %v", i.AsName, v.Module, i.Name))
				} else {
					add(jen.Commentf("import %q // %v", v.Module, i.Name))
				}
			}

		case *ast.Import:
			for _, i := range v.Names {
				if i.AsName != "" {
					add(jen.Commentf("import %s %q", i.AsName, i.Name))
					s.imports[string(i.AsName)] = string(i.Name)
				} else {
					add(jen.Commentf("import %q", i.Name))
					s.imports[string(i.Name)] = string(i.Name)
				}
			}

		case *ast.FunctionDef:
			var receiver jen.Code
			var returns jen.Code

			for _, d := range v.DecoratorList {
				add(jen.Commentf("// @%v\n", s.goExpr(d).GoString()))
			}

			fs := s.Push()

			arguments, recv := fs.goFunctionArguments(v.Args, classname != "")
			if recv != nil {
				receiver = jen.Params(goId(recv.Arg).Op("*").Id(classname))
			}
			if v.Returns != nil && !isNone(v.Returns) {
				returns = jen.Params(fs.goExprOrList(v.Returns))
			}

			stmt := jen.Func()
			if receiver != nil {
				if string(v.Name) == "__str__" {
					stmt.Add(receiver).Id("String")
					returns = jen.Params(jen.Id("string"))
				} else {
					stmt.Add(receiver).Add(goId(v.Name))
				}
			} else if s.level <= 1 {
				stmt.Add(goId(v.Name))
			} else {
				stmt = goId(v.Name).Op(":=").Func()
			}

			stmt.Params(arguments)
			if returns != nil {
				stmt.Add(returns)
			}
			stmt.Block(fs.parseBody("", v.Body))
			fs.Pop()
			add(stmt)

		case *ast.ClassDef:
			for _, d := range v.DecoratorList {
				add(jen.Commentf("@%v\n", s.goExpr(d).GoString()))
			}

			classdef := jen.Type().Add(goId(v.Name)).StructFunc(func(g *jen.Group) {
				cdefs := ""

				if len(v.Bases) > 0 || len(v.Keywords) > 0 {
					if len(v.Bases) > 0 {
						cdefs += " " + s.goExpr(v.Bases).GoString()
					}

					if len(v.Keywords) > 0 {
						cdefs += " " + s.goExpr(v.Keywords).GoString()
					}
				}

				g.Add(jen.Commentf("%v", cdefs))
			})

			add(classdef.Line())

			/***********************************
			  Here we should be expecting only:

			  - pass (and nothing else)
			  - string: should be a __doc__ string
			  - assignments (class variable)
			  - function definition (class methods)

			  Anything else should be an error.

			  So, we could convert:
			  - pass: empty struct (done)
			  - string: add comment to struct body
			  - assignements: struct fields (with value in comment)
			  - class methods: parse body and add to most outer scope
			  ***********************************/

			switch len(v.Body) {
			case 0:
				continue

			case 1:
				if _, ok := v.Body[0].(*ast.Pass); ok {
					continue
				}
			}

			/*
			   for _, pst := range v.Body {
			           switch v := pst.(type) {
			           case *ast.ExprStmt:  // error if not string
			               if str, ok := expr.Value.(*ast.Str); ok {
			               } else {
			                   // error
			               }

			           case *ast.Assign:
			               //

			           case *ast.FunctionDef:
			               //

			           default:
			               // error
			           }
			   }
			*/

			add(s.parseBody(string(v.Name), v.Body))

		case *ast.Assign:
			stmt := s.goExpr(v.Targets).Op("=").Add(s.goExpr(v.Value))
			if classname != "" {
				stmt = jen.Var().Commentf("/*%v*/", classname).Add(stmt)
			} else if s.newNames(v.Targets) {
				stmt = jen.Var().Add(stmt)
			}
			add(stmt)

		case *ast.AugAssign:
			add(s.goExpr(v.Target).Add(s.goOpExt(v.Op, "=")).Add(s.goExpr(v.Value)))

		case *ast.ExprStmt:
			switch xStmt := v.Value.(type) {
			case *ast.Yield:
				generator = true
				ret := jen.Null()
				if xStmt.Value != nil {
					ret = s.goExprOrList(xStmt.Value)
				}
				//add(jen.Commentf("yield %s", ret.GoString()))
				add(jen.Return(ret).Comment("yield"))

			case *ast.YieldFrom:
				generator = true
				ret := jen.Null()
				if xStmt.Value != nil {
					ret = s.goExprOrList(xStmt.Value)
				}
				//add(jen.Commentf("yield from %s", ret.GoString()))
				add(jen.Return(ret).Comment("yield from"))

			default:
				add(s.goExpr(v.Value)) //.Line()
			}

		case *ast.Pass:
			add(jen.Comment("pass"))

		case *ast.Break:
			add(jen.Break())

		case *ast.Continue:
			add(jen.Continue())

		case *ast.Return:
			if v.Value == nil {
				add(jen.Return())
			} else {
				add(jen.Return(s.goExprOrList(v.Value)))
			}

		case *ast.If:
			stmt := jen.If(s.goExpr(v.Test)).Block(s.parseBody("", v.Body))
			if len(v.Orelse) > 0 {
				if _, ok := v.Orelse[0].(*ast.If); ok && len(v.Orelse) == 1 {
					stmt.Else().Add(s.parseBody("", v.Orelse))
				} else {
					stmt.Else().Block(s.parseBody("", v.Orelse))
				}
			}
			add(stmt)

		case *ast.For:
			stmt := s.goFor(v.Target, v.Iter).Block(s.parseBody("", v.Body))
			if len(v.Orelse) > 0 {
				stmt.Else().Block(s.parseBody("", v.Orelse))
			}
			add(stmt)

		case *ast.While:
			stmt := jen.For(s.goExpr(v.Test))
			if k, ok := v.Test.(*ast.NameConstant); ok && k.Value == py.True {
				stmt = jen.For()
			}
			stmt = stmt.Block(s.parseBody("", v.Body))
			if len(v.Orelse) > 0 {
				stmt.Else().Block(s.parseBody("", v.Orelse))
			}
			add(stmt)

		case *ast.Try:
			stmt := jen.If(
				jen.Err().Op(":=").Func().Params().Params(goException).Block(
					jen.Comment("try"),
					s.parseBody("", v.Body),
				).Call(),
				jen.Err().Op("!=").Nil())

			body := jen.Null()

			if len(v.Handlers) > 0 {
				body = jen.Switch(jen.Err()).BlockFunc(func(g *jen.Group) {
					g.Add(jen.Comment("except"))

					for _, h := range v.Handlers {
						ch := jen.Case(s.goExpr(h.ExprType))
						if h.Name != "" {
							ch.Block(jen.Commentf("as %v", h.Name), s.parseBody("", h.Body))
						} else {
							ch.Block(s.parseBody("", h.Body))
						}

						g.Add(ch)
					}
				})
			}

			stmt.Block(body)

			if len(v.Orelse) > 0 {
				stmt.Else().Block(s.parseBody("", v.Orelse))
			}

			if len(v.Finalbody) > 0 {
				stmt.Line().Block(jen.Comment("finally"), s.parseBody("", v.Finalbody))
			}
			add(stmt)

		case *ast.Raise:
			stmt := jen.Return(jen.Id("RaisedException").Call(s.goExpr(v.Exc)))
			if v.Cause != nil {
				stmt.Commentf("cause: %v", s.goExpr(v.Cause).GoString())
			}
			add(stmt)

		case *ast.Assert:
			if v.Msg != nil {
				add(jen.Id("Assert").Call(s.goExpr(v.Test), s.goExpr(v.Msg)))
			} else {
				add(jen.Id("Assert").Call(s.goExpr(v.Test), jen.Lit("")))
			}

		case *ast.Global:
			add(jen.Commentf("global %v", s.goIdentifiers(v.Names).GoString()))

		case *ast.Nonlocal:
			add(jen.Commentf("nonlocal %v", s.goIdentifiers(v.Names).GoString()))

		case *ast.Delete:
			for _, t := range v.Targets {
				if st, ok := t.(*ast.Subscript); ok {
					if i, ok := st.Slice.(*ast.Index); ok {
						add(jen.Delete(s.goExpr(st.Value), s.goExpr(i.Value)))
					} else {
						log.Panicf("unexpected DELETE %#v", st)
					}
				} else {
					add(jen.Comment(unknown("DELETE", t).GoString()))
				}
			}

		case *ast.With:
			// We should really create an anonymous function
			// with a defer (that we can't really fill, but in a few cases)
			add(jen.BlockFunc(func(g *jen.Group) {
				g.Comment("with")

				for _, item := range v.Items {
					if item.OptionalVars != nil {
						g.Add(s.goExpr(item.OptionalVars).Op(":=").Add(s.goExpr(item.ContextExpr)))
					} else {
						g.Add(s.goExpr(item.ContextExpr))
					}
				}

				g.Line().Add(s.parseBody("", v.Body))
			}))

		default:
			add(jen.Comment(unknown("STMT", stmt).GoString()))
		}
	}

	s.level -= 1
	return parsed, stmts, generator
}

func main() {
	flag.IntVar(&debugLevel, "d", debugLevel, "Debug level 0-4")
	flag.BoolVar(&panicUnknown, "panic", panicUnknown, "panic on unknown expression, to get a stacktrace")
	flag.BoolVar(&verbose, "verbose", verbose, "print statement and expressions")
	flag.BoolVar(&mainpackage, "main", mainpackage, "generate a runnable application (main package)")
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

		fi, err := in.Stat()
		if err != nil {
			log.Fatal(err)
		}

		pname := strings.TrimSuffix(fi.Name(), ".py")
		if mainpackage {
			pname = "main"
		}

		tree, err := parser.Parse(in, path, "exec")
		if err != nil {
			log.Fatal(err)
		}

		m, ok := tree.(*ast.Module)
		if !ok {
			log.Fatal("expected Module, got", tree)
		}

		scope := NewScope()

		parsed, stmts, _ := scope.parseBodyEx("", m.Body)

		f := jen.NewFile(pname)
		f.PackageComment("generated by gopyr")
		f.Add(parsed)

		if false {
			f.Render(os.Stdout)
		} else {
			f.Render(ioutil.Discard)
			fmt.Println("// generated by gopyr")
			fmt.Println("package", pname)
			fmt.Println()
			f.RenderImports(os.Stdout)

			stmts = append(stmts, jen.Line())

			for _, s := range stmts {
				s.Render(os.Stdout)
			}
		}
	}
}

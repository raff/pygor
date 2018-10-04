package runtime

import "fmt"

type Any = interface{}
type Dict = map[string]Any
type List = []Any
type Tuple = []Any

func Assert(cond bool, message string) {
	if !cond {
		panic("AssertionError: " + message)
	}
}

type PyException struct {
	exc interface{}
}

func (e *PyException) Error() string {
	return fmt.Sprintf("PyException(%v)", e.exc)
}

func RaisedException(exc interface{}) PyException {
	return PyException{exc: exc}
}

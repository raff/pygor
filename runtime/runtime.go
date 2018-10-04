package runtime

import "fmt"

type any = interface{}
type dict = map[string]any
type list = []any
type tuple = []any

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

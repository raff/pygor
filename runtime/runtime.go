package runtime

import "fmt"

type dict = map[string]interface{}
type list = []interface{}
type tuple = []interface{}

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

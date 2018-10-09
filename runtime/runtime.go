package runtime

import "fmt"
import "strings"

type Any = interface{}
type Dict = map[string]Any
type List = []Any
type Tuple = []Any

func Assert(cond bool, message string) {
	if !cond {
		panic("AssertionError: " + message)
	}
}

func Contains(bag, value interface{}) bool {
	switch c := bag.(type) {
	case Dict:
		if s, ok := value.(string); ok {
			_, ok = c[s]
			return ok
		}

	case List:
		for _, v := range c {
			if v == value {
				return true
			}
		}

	case Tuple:
		for _, v := range c {
			if v == value {
				return true
			}
		}

	case string:
		if s, ok := value.(string); ok {
			return strings.Contains(c, s)
		}
	}

	return false
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

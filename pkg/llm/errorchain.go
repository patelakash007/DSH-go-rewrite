//

//

package llm

import (
	"fmt"
	"reflect"
	"strings"
)

//

func RenderErrorChain(v any) string {
	path := make(map[error]bool)
	return renderChain(v, path)
}

func renderChain(cur any, path map[error]bool) string {
	err, ok := cur.(error)
	if !ok {

		if s, ok2 := messageOf(cur); ok2 {
			return s
		}
		return fmt.Sprint(cur)
	}
	if path[err] {
		return "<circular cause>"
	}
	path[err] = true
	defer delete(path, err)

	msg := err.Error()
	if msg == "" {
		msg = typeNameOf(err)
	}

	if mw, ok := err.(interface{ Unwrap() []error }); ok {
		members := mw.Unwrap()
		parts := make([]string, 0, len(members))
		for _, m := range members {
			if m != nil {
				parts = append(parts, renderChain(m, path))
			}
		}
		if len(parts) == 0 {
			return msg
		}
		return msg + " [" + strings.Join(parts, "; ") + "]"
	}

	if uw, ok := err.(interface{ Unwrap() error }); ok {
		cause := uw.Unwrap()
		if cause != nil {
			causeText := renderChain(cause, path)
			if causeText != "" && causeText != msg {
				return msg + ": " + causeText
			}
		}
	}

	return msg
}

func messageOf(v any) (string, bool) {
	rv := reflect.ValueOf(v)
	if !rv.IsValid() {
		return "", false
	}
	if rv.Kind() == reflect.Ptr {
		if rv.IsNil() {
			return "", false
		}
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return "", false
	}
	f := rv.FieldByName("Message")
	if f.IsValid() && f.Kind() == reflect.String {
		return f.String(), true
	}
	return "", false
}

func typeNameOf(err error) string {
	t := reflect.TypeOf(err)
	if t == nil {
		return "error"
	}
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Name() == "" {
		return "error"
	}
	return t.Name()
}

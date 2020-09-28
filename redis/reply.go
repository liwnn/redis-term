package redis

import (
	"errors"
	"fmt"
)

// Result reply
type Result struct {
	object *Object
}

// NewResult new
func NewResult(object *Object) *Result {
	return &Result{
		object: object,
	}
}

// List returns a string slice.
func (r *Result) List() ([]string, error) {
	if r.object.Type == Err {
		return nil, r.object.val.(error)
	}
	elems, ok := r.object.val.([]*Object)
	if !ok {
		return nil, fmt.Errorf("convert to []interface{}")
	}
	if len(elems) == 0 {
		return nil, errors.New("(empty list or set)")
	}

	s := make([]string, 0, len(elems))
	for _, ele := range elems {
		s = append(s, string(ele.val.([]byte)))
	}
	return s, nil
}

//func (r Reply) String(writer io.Writer) error {
//    switch r.object.Type {
//    case SimpleStr:
//        d := r.object.val
//        if isText(d) {
//            fmt.Fprintf(writer, "%s\n", string(d))
//        } else {
//            for _, b := range d {
//                //s := strconv.FormatInt(int64(b&0xff), 16)
//                fmt.Fprintf(writer, "\\x%02x", b)
//            }
//            fmt.Fprintf(writer, "\n")
//        }
//        return nil
//    default:
//        return fmt.Errorf("convert %s to string", r.object)
//    }
//}

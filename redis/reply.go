package redis

import (
	"errors"
	"fmt"
	"strconv"
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

// Type return type.
func (r *Result) Type() Type {
	return r.object.Type
}

// IsNil return if object is nil
func (r *Result) IsNil() bool {
	return r.object.Type == Nil
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

// String return string.
func (r *Result) String() string {
	switch r.object.Type {
	case Err:
		return r.object.val.(error).Error()
	case SimpleStr:
		return string(r.object.val.([]byte))
	case BulkStr:
		return string(r.object.val.([]byte))
	}
	return ""
}

// Byte returns []byte.
func (r *Result) Byte() []byte {
	if r.object.Type == Err {
		return nil
	}
	if !(r.object.Type == SimpleStr || r.object.Type == BulkStr) {
		return nil
	}
	return r.object.val.([]byte)
}

// Int return int
func (r *Result) Int() (int, error) {
	if r.object.Type != Int {
		return 0, errors.New("not int typ")
	}
	v, ok := r.object.val.([]byte)
	if !ok {
		return 0, errors.New("not int")
	}
	return strconv.Atoi(string(v))
}

// ToArray to array
func (r *Result) ToArray() []*Result {
	if r.object.Type != Array {
		return nil
	}

	t := r.object.val.([]*Object)
	ret := make([]*Result, 0, len(t))
	for _, v := range t {
		ret = append(ret, NewResult(v))
	}
	return ret
}

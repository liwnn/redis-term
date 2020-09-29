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

package redis

import (
	"errors"
	"fmt"
	"strconv"
)

// Reply reply
type Reply struct {
	object *Object
}

// NewReply new
func NewReply(object *Object) *Reply {
	switch object.Type {
	case SimpleStr:
	case Err:
	case Int:
	case BulkStr:
	case Array:
	case Nil:
	}
	return &Reply{
		object: object,
	}
}

// Type return type.
func (r *Reply) Type() Type {
	return r.object.Type
}

// IsNil return if object is nil
func (r *Reply) IsNil() bool {
	return r.object.Type == Nil
}

// List returns a string slice.
func (r *Reply) List() ([]string, error) {
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
func (r *Reply) String() string {
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
func (r *Reply) Byte() []byte {
	if r.object.Type == Err {
		return nil
	}
	if !(r.object.Type == SimpleStr || r.object.Type == BulkStr) {
		return nil
	}
	return r.object.val.([]byte)
}

// Int return int
func (r *Reply) Int() (int, error) {
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
func (r *Reply) ToArray() []*Reply {
	if r.object.Type != Array {
		return nil
	}

	t := r.object.val.([]*Object)
	ret := make([]*Reply, 0, len(t))
	for _, v := range t {
		ret = append(ret, NewReply(v))
	}
	return ret
}

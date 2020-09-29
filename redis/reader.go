package redis

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
)

// const
const (
	SIMPLE_STRING = '+'
	BULK_STRING   = '$'
	INTEGER       = ':'
	ARRAY         = '*'
	ERROR         = '-'
)

// ErrInvalidSyntax err
var ErrInvalidSyntax = errors.New("resp: invalid syntax")

// Type is the message type.
type Type int

// type
const (
	SimpleStr Type = iota
	Err
	Int
	BulkStr
	Array
	Nil
)

// Object is the reply.
type Object struct {
	Type
	val interface{}
}

// NewObject new
func NewObject(t Type, val interface{}) *Object {
	return &Object{
		Type: t,
		val:  val,
	}
}

// RESPReader reader
type RESPReader struct {
	*bufio.Reader
}

// NewReader new
func NewReader(reader io.Reader) *RESPReader {
	return &RESPReader{
		Reader: bufio.NewReaderSize(reader, 32*1024),
	}
}

// ReadObject read
func (r *RESPReader) ReadObject() (*Object, error) {
	line, err := r.readLine()
	if err != nil {
		return nil, err
	}

	switch line[0] {
	case SIMPLE_STRING: // +OK\r\n
		return NewObject(SimpleStr, line[1:len(line)-2]), nil
	case ERROR:
		return NewObject(Err, line[1:len(line)-2]), fmt.Errorf("(error) %s", line[1:len(line)-2])
	case INTEGER: // :99\r\n  -ERR unknown command 'GETT'\r\n
		return NewObject(Int, line[1:len(line)-2]), nil
	case BULK_STRING: // $13\r\nHello, World!\r\n
		b, err := r.readBulkString(line[:len(line)-2])
		return NewObject(BulkStr, b), err
	case ARRAY: // *3\r\n$3\r\nSET\r\n$5\r\nmykey\r\n$8\r\nmy value\r\n
		return r.readArray(line[:len(line)-2])
	default:
		return nil, ErrInvalidSyntax
	}
}

func (r *RESPReader) readBulkString(line []byte) ([]byte, error) {
	count, err := r.getCount(line)
	if err != nil {
		return nil, err
	}

	if count == -1 {
		return line, nil
	}

	buff := make([]byte, count+2)
	_, err = io.ReadFull(r, buff)
	if err != nil {
		return nil, err
	}

	buff = buff[:count]
	return buff, nil
}

func (r *RESPReader) getCount(line []byte) (int, error) {
	return strconv.Atoi(string(line[1:]))
}

func (r *RESPReader) readArray(line []byte) (*Object, error) {
	count, err := r.getCount(line)
	if err != nil {
		return nil, err
	}
	var elems = make([]*Object, 0, count)
	for i := 0; i < count; i++ {
		buf, err := r.ReadObject()
		if err != nil {
			return nil, err
		}
		elems = append(elems, buf)
	}
	return NewObject(Array, elems), nil
}

func (r *RESPReader) readLine() (line []byte, err error) {
	line, err = r.ReadBytes('\n')
	if err != nil {
		return nil, err
	}

	if len(line) > 1 && line[len(line)-2] == '\r' {
		return line, nil
	}
	return nil, ErrInvalidSyntax
}

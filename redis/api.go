package redis

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
)

var (
	arrayPrefixSlice      = []byte{'*'}
	bulkStringPrefixSlice = []byte{'$'}
	lineEndingSlice       = []byte{'\r', '\n'}
)

// RESPWriter w
type RESPWriter struct {
	*bufio.Writer
}

// NewRESPWriter new
func NewRESPWriter(writer io.Writer) *RESPWriter {
	return &RESPWriter{
		Writer: bufio.NewWriter(writer),
	}
}

// WriteCommand write
// @param args - All Redis commands are sent as arrays of bulk strings. *3\r\n$3\r\nSET\r\n$5\r\nmykey\r\n$8\r\nmy value\r\n
func (w *RESPWriter) WriteCommand(args ...string) (err error) {
	w.Write(arrayPrefixSlice)
	w.WriteString(strconv.Itoa(len(args)))
	w.Write(lineEndingSlice)

	for _, arg := range args {
		w.Write(bulkStringPrefixSlice)
		w.WriteString(strconv.Itoa(len(arg)))
		w.Write(lineEndingSlice)
		w.WriteString(arg)
		w.Write(lineEndingSlice)
	}

	return w.Flush()
}

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
func (r *RESPReader) ReadObject() (interface{}, error) {
	line, err := r.readLine()
	if err != nil {
		return nil, err
	}

	switch line[0] {
	case SIMPLE_STRING, INTEGER: // +OK\r\n  :99\r\n  -ERR unknown command 'GETT'\r\n
		return line[1:], nil
	case ERROR:
		return nil, fmt.Errorf("(error) %s", line[1:])
	case BULK_STRING: // $13\r\nHello, World!\r\n
		return r.readBulkString(line)
	case ARRAY: // *3\r\n$3\r\nSET\r\n$5\r\nmykey\r\n$8\r\nmy value\r\n
		return r.readArray(line)
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

func (r *RESPReader) readArray(line []byte) (interface{}, error) {
	count, err := r.getCount(line)
	if err != nil {
		return nil, err
	}
	var elems []interface{}
	for i := 0; i < count; i++ {
		buf, err := r.ReadObject()
		if err != nil {
			return nil, err
		}
		elems = append(elems, buf)
	}
	return elems, nil
}

func (r *RESPReader) readLine() (line []byte, err error) {
	line, err = r.ReadBytes('\n')
	if err != nil {
		return nil, err
	}

	if len(line) > 1 && line[len(line)-2] == '\r' {
		line = line[:len(line)-2]
		return line, nil
	}
	return nil, ErrInvalidSyntax
}

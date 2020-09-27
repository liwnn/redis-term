package redis

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
)

// Command command
type Command struct {
	cmd  []string
	data interface{}
}

// NewCommand new
func NewCommand(cmd []string) *Command {
	return &Command{
		cmd: cmd,
	}
}

// Result result
type Result struct {
	data interface{}
}

// NewResult new
func NewResult(data interface{}) *Result {
	return &Result{
		data: data,
	}
}

func isText(d []byte) bool {
	if bytes.Index(d, []byte{0}) != -1 {
		return false
	}
	return true
}

func (r Result) String(writer io.Writer) error {
	switch r.data.(type) {
	case []byte:
		d, ok := r.data.([]byte)
		if !ok {
			return fmt.Errorf("convert to []byte")
		}
		if isText(d) {
			fmt.Fprintf(writer, "%s\n", string(d))
		} else {
			for _, b := range d {
				//s := strconv.FormatInt(int64(b&0xff), 16)
				fmt.Fprintf(writer, "\\x%02x", b)
			}
			fmt.Fprintf(writer, "\n")
		}
		return nil
	case []interface{}:
		d, ok := r.data.([]interface{})
		if !ok {
			return fmt.Errorf("convert to []interface{}")
		}
		if len(d) == 0 {
			fmt.Fprintf(writer, "(empty list or set)\r\n")
			return nil
		}
		for i, ele := range d {
			fmt.Fprintf(writer, "%v", i+1)
			//writer.WriteString(strconv.Itoa(i + 1))
			fmt.Fprintf(writer, ") \"")
			b := ele.([]byte)
			fmt.Fprintf(writer, "%v", string(b))
			fmt.Fprintf(writer, "\"")
			fmt.Fprintf(writer, "\r\n")
		}
		return nil
	default:
		return fmt.Errorf("convert %s to string", r.data)
	}
}

// IsString is string same
func (r Result) IsString(s string) bool {
	b, ok := r.data.([]byte)
	if !ok {
		return false
	}
	return string(b) == s
}

// Client client
type Client struct {
	conn   net.Conn
	reader *RESPReader
	writer *RESPWriter

	index int
}

// NewClient new
func NewClient(conn net.Conn) *Client {
	rr := NewReader(conn)
	ww := NewRESPWriter(conn)
	return &Client{
		conn:   conn,
		reader: rr,
		writer: ww,
	}
}

// Do do
func (r *Client) Do(cmd ...string) (*Result, error) {
	if len(cmd) == 0 {
		return nil, fmt.Errorf("empty")
	}

	if err := r.writer.WriteCommand(cmd...); err != nil {
		return nil, err
	}
	o, err := r.reader.ReadObject()
	if err != nil {
		return nil, err
	}

	switch strings.ToUpper(cmd[0]) {
	case "SELECT":
		index, err := strconv.Atoi(cmd[1])
		if err != nil {
			return nil, err
		}
		r.index = index
	}

	return NewResult(o), nil
}

// Close the conn
func (r *Client) Close() {
	r.conn.Close()
}

// GetIndex get index
func (r Client) GetIndex() int {
	return r.index
}

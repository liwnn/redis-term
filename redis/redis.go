package redis

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

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

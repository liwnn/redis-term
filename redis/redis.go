package redis

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

// Client client
type Client struct {
	conn   net.Conn
	reader *RESPReader
	writer *RESPWriter

	timeout time.Duration
	index   int
}

// NewClient new
func NewClient(conn net.Conn) *Client {
	rr := NewReader(conn)
	ww := NewRESPWriter(conn)
	return &Client{
		conn:    conn,
		reader:  rr,
		writer:  ww,
		timeout: time.Second * 3,
	}
}

// Do do
func (r *Client) Do(cmd ...string) (*Result, error) {
	if len(cmd) == 0 {
		return nil, fmt.Errorf("empty")
	}

	if r.timeout > 0 {
		r.conn.SetWriteDeadline(time.Now().Add(r.timeout))
	}
	if err := r.writer.WriteCommand(cmd...); err != nil {
		return nil, err
	}

	if r.timeout > 0 {
		r.conn.SetReadDeadline(time.Now().Add(r.timeout))
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

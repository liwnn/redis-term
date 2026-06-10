package redis

import (
	"net"
	"time"
)

// Client client
type Client struct {
	conn   net.Conn
	reader *RESPReader
	writer *RESPWriter

	timeout time.Duration
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
func (r *Client) Do(key string, cmd ...string) (*Reply, error) {
	if r.timeout > 0 {
		r.conn.SetWriteDeadline(time.Now().Add(r.timeout))
	}
	if err := r.writer.WriteCommand(key, cmd...); err != nil {
		return nil, err
	}

	if r.timeout > 0 {
		r.conn.SetReadDeadline(time.Now().Add(r.timeout))
	}
	o, err := r.reader.ReadObject()
	if err != nil {
		return nil, err
	}
	return NewReply(o), nil
}

// Close the conn
func (r *Client) Close() {
	r.conn.Close()
}

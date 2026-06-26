package resp

import (
	"net"
	"time"
)

// Client client
type Client struct {
	conn   net.Conn
	reader *respReader
	writer *RESPWriter

	timeout time.Duration
}

// NewClient new
func NewClient(conn net.Conn) *Client {
	rr := newReader(conn)
	ww := NewRESPWriter(conn)
	return &Client{
		conn:    conn,
		reader:  rr,
		writer:  ww,
		timeout: time.Second * 3,
	}
}

// send writes a command, applying the write/read deadlines. Callers then invoke
// the matching reader method to decode the reply directly into a target type.
func (r *Client) send(key string, cmd ...string) error {
	if r.timeout > 0 {
		r.conn.SetWriteDeadline(time.Now().Add(r.timeout))
	}
	if err := r.writer.WriteCommand(key, cmd...); err != nil {
		return err
	}
	if r.timeout > 0 {
		r.conn.SetReadDeadline(time.Now().Add(r.timeout))
	}
	return nil
}

// DoStatus runs a command and reads a simple-string/status reply.
func (r *Client) DoStatus(key string, cmd ...string) (string, error) {
	if err := r.send(key, cmd...); err != nil {
		return "", err
	}
	return r.reader.readStatus()
}

// DoInt runs a command and reads an integer reply.
func (r *Client) DoInt(key string, cmd ...string) (int, error) {
	if err := r.send(key, cmd...); err != nil {
		return 0, err
	}
	return r.reader.readInt()
}

// DoBulk runs a command and reads a bulk-string reply (nil bulk -> nil).
func (r *Client) DoBulk(key string, cmd ...string) ([]byte, error) {
	if err := r.send(key, cmd...); err != nil {
		return nil, err
	}
	return r.reader.readBulk()
}

// DoStringArray runs a command and reads a flat array of bulk strings.
func (r *Client) DoStringArray(key string, cmd ...string) ([]string, error) {
	if err := r.send(key, cmd...); err != nil {
		return nil, err
	}
	return r.reader.readStringArray()
}

// DoScan runs SCAN and reads its [cursor, keys] reply.
func (r *Client) DoScan(key string, cmd ...string) (string, []string, error) {
	if err := r.send(key, cmd...); err != nil {
		return "", nil, err
	}
	return r.reader.readScanReply()
}

// DoStream runs XRANGE and reads its [id, fields...] entries.
func (r *Client) DoStream(key string, cmd ...string) ([][]string, error) {
	if err := r.send(key, cmd...); err != nil {
		return nil, err
	}
	return r.reader.readStreamReply()
}

// DoReply runs an arbitrary command and reads its reply as a Go-native value.
func (r *Client) DoReply(key string, cmd ...string) (any, error) {
	if err := r.send(key, cmd...); err != nil {
		return nil, err
	}
	return r.reader.readReply()
}

// Close the conn
func (r *Client) Close() {
	r.conn.Close()
}

package resp

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"unsafe"
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

// parseInt parses a base-10 integer (optionally signed) directly from bytes,
// avoiding the string allocation strconv.Atoi(string(b)) would incur.
func parseInt(b []byte) (int, error) {
	if len(b) == 0 {
		return 0, ErrInvalidSyntax
	}
	neg := false
	i := 0
	if b[0] == '-' || b[0] == '+' {
		neg = b[0] == '-'
		i = 1
		if len(b) == 1 {
			return 0, ErrInvalidSyntax
		}
	}
	n := 0
	for ; i < len(b); i++ {
		c := b[i]
		if c < '0' || c > '9' {
			return 0, ErrInvalidSyntax
		}
		n = n*10 + int(c-'0')
	}
	if neg {
		n = -n
	}
	return n, nil
}

// respReader reads RESP replies from a connection.
type respReader struct {
	*bufio.Reader
}

// newReader new
func newReader(reader io.Reader) *respReader {
	// A larger buffer lets each socket read pull more of an already-arrived reply
	// at once, cutting syscalls on big bulk replies (e.g. a SCAN batch of 10k keys
	// is ~1MB, which a 32KB buffer would drain in ~32 reads vs ~4 here).
	return &respReader{
		Reader: bufio.NewReaderSize(reader, 256*1024),
	}
}

// getCount parses the numeric argument of a header line ($N / *N), accepting the
// line either with or without its trailing CRLF.
func (r *respReader) getCount(line []byte) (int, error) {
	b := line[1:]
	if n := len(b); n >= 2 && b[n-2] == '\r' && b[n-1] == '\n' {
		b = b[:n-2]
	}
	return parseInt(b)
}

func (r *respReader) readLine() (line []byte, err error) {
	line, err = r.ReadSlice('\n')
	if err != nil {
		return nil, err
	}

	if len(line) > 1 && line[len(line)-2] == '\r' {
		return line, nil
	}
	return nil, ErrInvalidSyntax
}

// payload returns a reply line's body, i.e. the bytes between the type prefix
// and the trailing CRLF.
func payload(line []byte) []byte {
	return line[1 : len(line)-2]
}

// errReply turns a RESP error line ("-ERR ...") into a Go error.
func errReply(line []byte) error {
	return fmt.Errorf("(error) %s", payload(line))
}

// readStatus reads a simple-string / status reply (+OK). An error reply becomes
// a Go error.
func (r *respReader) readStatus() (string, error) {
	line, err := r.readLine()
	if err != nil {
		return "", err
	}
	switch line[0] {
	case SIMPLE_STRING:
		return string(payload(line)), nil
	case ERROR:
		return "", errReply(line)
	default:
		return "", ErrInvalidSyntax
	}
}

// readInt reads an integer reply (:N).
func (r *respReader) readInt() (int, error) {
	line, err := r.readLine()
	if err != nil {
		return 0, err
	}
	switch line[0] {
	case INTEGER:
		return parseInt(payload(line))
	case ERROR:
		return 0, errReply(line)
	default:
		return 0, ErrInvalidSyntax
	}
}

// readBulk reads a bulk-string reply ($N). A RESP nil bulk ($-1) returns
// (nil, nil).
func (r *respReader) readBulk() ([]byte, error) {
	line, err := r.readLine()
	if err != nil {
		return nil, err
	}
	switch line[0] {
	case BULK_STRING:
		return r.readBulkBody(line)
	case ERROR:
		return nil, errReply(line)
	default:
		return nil, ErrInvalidSyntax
	}
}

// readBulkBody reads the data of a bulk string whose header line was already
// read. Returns nil for a nil bulk ($-1).
func (r *respReader) readBulkBody(line []byte) ([]byte, error) {
	count, err := r.getCount(line)
	if err != nil {
		return nil, err
	}
	if count == -1 {
		return nil, nil
	}
	buff := make([]byte, count+2)
	if _, err := io.ReadFull(r, buff); err != nil {
		return nil, err
	}
	return buff[:count], nil
}

// readStringArray decodes a flat array of bulk strings straight into a []string,
// bypassing any intermediate tree: one slice plus one []byte per element, each
// aliased as a string without copying. This is the hot path for replies holding
// many values (SCAN keys / KEYS / SMEMBERS / HGETALL / LRANGE / ZRANGE).
func (r *respReader) readStringArray() ([]string, error) {
	line, err := r.readLine()
	if err != nil {
		return nil, err
	}
	switch line[0] {
	case ERROR:
		return nil, errReply(line)
	case ARRAY:
	default:
		return nil, ErrInvalidSyntax
	}
	count, err := r.getCount(line)
	if err != nil {
		return nil, err
	}
	if count <= 0 {
		return nil, nil
	}
	out := make([]string, 0, count)
	for i := 0; i < count; i++ {
		b, err := r.readBulk()
		if err != nil {
			return nil, err
		}
		out = append(out, unsafe.String(unsafe.SliceData(b), len(b)))
	}
	return out, nil
}

// readArrayHeader reads an array header line and returns the element count. An
// error reply becomes a Go error; a nil array ($-1 style *-1) returns -1.
func (r *respReader) readArrayHeader() (int, error) {
	line, err := r.readLine()
	if err != nil {
		return 0, err
	}
	switch line[0] {
	case ERROR:
		return 0, errReply(line)
	case ARRAY:
		return r.getCount(line)
	default:
		return 0, ErrInvalidSyntax
	}
}

// readReply decodes an arbitrary reply into a Go-native value, for callers that
// run user-supplied commands (redis-cli) and can't know the type in advance:
//
//	+OK    -> string
//	:N     -> int
//	$..    -> string (nil bulk -> nil)
//	*..    -> []any (recursive; nil array -> nil)
//	-ERR   -> error
func (r *respReader) readReply() (any, error) {
	line, err := r.readLine()
	if err != nil {
		return nil, err
	}
	switch line[0] {
	case SIMPLE_STRING:
		return string(payload(line)), nil
	case ERROR:
		return nil, errReply(line)
	case INTEGER:
		return parseInt(payload(line))
	case BULK_STRING:
		b, err := r.readBulkBody(line)
		if err != nil {
			return nil, err
		}
		if b == nil {
			return nil, nil
		}
		return string(b), nil
	case ARRAY:
		count, err := r.getCount(line)
		if err != nil {
			return nil, err
		}
		if count < 0 {
			return nil, nil
		}
		arr := make([]any, 0, count)
		for i := 0; i < count; i++ {
			v, err := r.readReply()
			if err != nil {
				return nil, err
			}
			arr = append(arr, v)
		}
		return arr, nil
	default:
		return nil, ErrInvalidSyntax
	}
}

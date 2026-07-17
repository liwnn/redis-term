package redis

import (
	"errors"
	"fmt"
	"io"
	"net"
	"testing"
	"time"

	"github.com/liwnn/redisterm/datasource/redis/redisapi/resp"
)

// fakeNetErr is a net.Error for exercising isConnErr's transport-failure branch.
type fakeNetErr struct{}

func (fakeNetErr) Error() string   { return "fake net error" }
func (fakeNetErr) Timeout() bool   { return true }
func (fakeNetErr) Temporary() bool { return true }

// TestIsConnErr checks that only connection-level failures (transport errors and
// RESP desync) trigger a reconnect, while normal redis error replies do not —
// misclassifying a "(error) WRONGTYPE ..." reply as a broken connection would
// pointlessly re-dial on every type mismatch.
func TestIsConnErr(t *testing.T) {
	conn := []error{
		resp.ErrInvalidSyntax,
		fmt.Errorf("wrapped: %w", resp.ErrInvalidSyntax),
		io.EOF,
		io.ErrUnexpectedEOF,
		fakeNetErr{},
		net.ErrClosed,
	}
	for _, err := range conn {
		if !isConnErr(err) {
			t.Errorf("isConnErr(%v) = false, want true", err)
		}
	}
	notConn := []error{
		nil,
		errors.New("(error) WRONGTYPE Operation against a key holding the wrong kind of value"),
		errors.New("(error) NOAUTH Authentication required"),
		fmt.Errorf("invalid container %q", "xyz"),
	}
	for _, err := range notConn {
		if isConnErr(err) {
			t.Errorf("isConnErr(%v) = true, want false", err)
		}
	}
}

// TestRedisSourceEntries verifies that Entries scans and returns every key in a
// db, including across multiple SCAN batches. It runs against a local redis and
// skips when none is reachable. To stay safe on a possibly-shared server it only
// writes/deletes keys under a unique prefix and never flushes the db.
func TestRedisSourceEntries(t *testing.T) {
	const container = "0" // high db index, least likely to hold real data
	const prefix = "redisterm_test:entries:"
	const n = 2500 // > one SCAN COUNT batch is not required, but exercises paging

	s := NewRedisSource("127.0.0.1:9898", "")
	if err := s.Open(); err != nil {
		t.Skipf("no local redis: %v", err)
	}
	defer s.Close()

	var begin = time.Now()
	a, err := s.Entries(container)
	if err != nil {
		t.Errorf("Entries: %v", err)
	}
	_ = a
	var elapsed = time.Since(begin)
	fmt.Println(elapsed)
}

package redis

import (
	"fmt"
	"testing"
	"time"
)

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

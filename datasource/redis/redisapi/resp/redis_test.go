package resp

import (
	"net"
	"testing"
)

// TestRedis_Do exercises a real round-trip against a local redis. It is skipped
// when no server is listening, so the package's parser tests still run offline.
func TestRedis_Do(t *testing.T) {
	conn, err := net.Dial("tcp", "127.0.0.1:6379")
	if err != nil {
		t.Skip("no local redis on 127.0.0.1:6379")
	}
	client := NewClient(conn)
	defer client.Close()

	if _, err := client.DoStatus("SET", "redis_do_test_key", "v"); err != nil {
		t.Skipf("redis not usable without setup: %v", err) // e.g. NOAUTH
	}
	b, err := client.DoBulk("GET", "redis_do_test_key")
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "v" {
		t.Fatalf("got %q want v", b)
	}
}

package redis

import (
	"net"
	"testing"
)

func TestRedis_Do(t *testing.T) {
	conn, _ := net.Dial("tcp", "127.0.0.1:9898")
	client := NewClient(conn)

	r, err := client.Do("get", "bdafasd")
	if err != nil {
		t.Error(err)
	}
	mm := r.Byte()
	t.Error(string(mm))
}

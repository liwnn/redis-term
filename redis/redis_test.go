package redis

import (
	"fmt"
	"net"
	"testing"
)

func TestRedis_Do(t *testing.T) {
	conn, _ := net.Dial("tcp", "127.0.0.1:9898")
	client := NewClient(conn)

	r, err := client.Do("get", "b")
	if err != nil {
		t.Error(err)
	}
	mm := r.Byte()
	fmt.Println(r, mm)
}

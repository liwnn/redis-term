package main

import "testing"

func TestRedis(t *testing.T) {
	client := NewRedis("127.0.0.1:9898")
	defer client.Close()
	t.Error(client.Type("e:1000"))
}

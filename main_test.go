package main

import "testing"

func TestRedis(t *testing.T) {
	client := NewRedis("127.0.0.1:6379")
	defer client.Close()
	t.Error(client.Select(1))
}

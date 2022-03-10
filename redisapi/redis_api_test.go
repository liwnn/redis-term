package redisapi

import "testing"

func TestRedis(t *testing.T) {
	client := NewRedis("127.0.0.1:9898", "")
	defer client.Close()
	s := client.Get("game:dy:schedule")
	if isText([]byte(s)) {
		t.Error(s)
	}
	_, sss := client.Scan("0", "*", 10000)
	if sss == nil {

	}
}

package redisapi

import "testing"

func TestRedis(t *testing.T) {
	client, err := NewRedis("127.0.0.1:9898", "", nil)
	if err != nil {
		return
	}
	defer client.Close()
	client.Get("game:dy:schedule")
	_, sss, err := client.Scan("0", "*", 100)
	if sss == nil {

	}
}

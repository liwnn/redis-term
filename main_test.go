package redisterm

import "testing"

func TestRedis(t *testing.T) {
	client := NewRedis("127.0.0.1:9898")
	defer client.Close()
	s := client.Get("u:1727310005024170")
	t.Error(s)
}

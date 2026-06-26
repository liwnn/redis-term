package resp

import (
	"bufio"
	"strings"
	"testing"
)

func reader(s string) *respReader {
	return &respReader{Reader: bufio.NewReader(strings.NewReader(s))}
}

func TestReadStatus(t *testing.T) {
	s, err := reader("+OK\r\n").readStatus()
	if err != nil || s != "OK" {
		t.Fatalf("got %q %v", s, err)
	}
	if _, err := reader("-ERR boom\r\n").readStatus(); err == nil {
		t.Fatal("expected error")
	}
}

func TestReadIntMethod(t *testing.T) {
	cases := map[string]int{":0\r\n": 0, ":42\r\n": 42, ":-3\r\n": -3}
	for in, want := range cases {
		got, err := reader(in).readInt()
		if err != nil || got != want {
			t.Fatalf("%q: got %d %v want %d", in, got, err, want)
		}
	}
}

func TestReadBulkMethod(t *testing.T) {
	b, err := reader("$5\r\nhello\r\n").readBulk()
	if err != nil || string(b) != "hello" {
		t.Fatalf("got %q %v", b, err)
	}
	nb, err := reader("$-1\r\n").readBulk()
	if err != nil || nb != nil {
		t.Fatalf("nil bulk: got %v %v", nb, err)
	}
}

func TestReadStringArrayMethod(t *testing.T) {
	got, err := reader("*3\r\n$1\r\na\r\n$2\r\nbb\r\n$3\r\nccc\r\n").readStringArray()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0] != "a" || got[1] != "bb" || got[2] != "ccc" {
		t.Fatalf("got %v", got)
	}
	// empty array
	empty, err := reader("*0\r\n").readStringArray()
	if err != nil || empty != nil {
		t.Fatalf("empty: got %v %v", empty, err)
	}
}

func TestReadScanReplyMethod(t *testing.T) {
	cursor, keys, err := reader("*2\r\n$2\r\n17\r\n*2\r\n$1\r\na\r\n$1\r\nb\r\n").readScanReply()
	if err != nil {
		t.Fatal(err)
	}
	if cursor != "17" {
		t.Fatalf("cursor = %q", cursor)
	}
	if len(keys) != 2 || keys[0] != "a" || keys[1] != "b" {
		t.Fatalf("keys = %v", keys)
	}
}

func TestReadStreamReplyMethod(t *testing.T) {
	// one entry: id "1-0", fields k1=v1 k2=v2
	in := "*1\r\n*2\r\n$3\r\n1-0\r\n*4\r\n$2\r\nk1\r\n$2\r\nv1\r\n$2\r\nk2\r\n$2\r\nv2\r\n"
	rows, err := reader(in).readStreamReply()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %v", rows)
	}
	want := []string{"1-0", "k1", "v1", "k2", "v2"}
	for i, w := range want {
		if rows[0][i] != w {
			t.Fatalf("row = %v", rows[0])
		}
	}
}

func TestReadReplyGeneric(t *testing.T) {
	if v, _ := reader("+OK\r\n").readReply(); v != "OK" {
		t.Fatalf("status: %v", v)
	}
	if v, _ := reader(":7\r\n").readReply(); v != 7 {
		t.Fatalf("int: %v", v)
	}
	if v, _ := reader("$3\r\nfoo\r\n").readReply(); v != "foo" {
		t.Fatalf("bulk: %v", v)
	}
	if v, _ := reader("$-1\r\n").readReply(); v != nil {
		t.Fatalf("nil bulk: %v", v)
	}
	v, err := reader("*2\r\n$1\r\na\r\n:5\r\n").readReply()
	if err != nil {
		t.Fatal(err)
	}
	arr, ok := v.([]any)
	if !ok || len(arr) != 2 || arr[0] != "a" || arr[1] != 5 {
		t.Fatalf("array: %v", v)
	}
	if _, err := reader("-ERR bad\r\n").readReply(); err == nil {
		t.Fatal("expected error")
	}
}

func TestParseInt(t *testing.T) {
	ok := map[string]int{"0": 0, "7": 7, "-7": -7, "+7": 7, "2080000": 2080000}
	for in, want := range ok {
		got, err := parseInt([]byte(in))
		if err != nil || got != want {
			t.Fatalf("parseInt(%q) = %d, %v; want %d", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "-", "+", "1a", "a1", "1 2"} {
		if _, err := parseInt([]byte(bad)); err == nil {
			t.Fatalf("parseInt(%q) expected error", bad)
		}
	}
}

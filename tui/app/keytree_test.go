package app

import (
	"strconv"
	"testing"
)

func TestAddKey(t *testing.T) {
	tree := NewDataTree("root")
	tree.AddKey("a")
	tree.AddKey("a")
	tree.AddKey("a:b:c")
	tree.AddKey("a:c")
}

func BenchmarkAddKey(b *testing.B) {
	tree := NewDataTree("root")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tree.AddKey("a:b:" + strconv.Itoa(i))
	}
}

// benchKeys builds a realistic 2M-key set: a handful of top prefixes, each with
// an id segment, mirroring the production "user:123" / "session:abc" shape that
// makes addKey cost ~419ms on the real db.
func benchKeys(n int) []string {
	prefixes := []string{"user", "session", "order", "cache", "job", "event", "item", "log"}
	keys := make([]string, n)
	for i := 0; i < n; i++ {
		p := prefixes[i%len(prefixes)]
		keys[i] = p + ":" + strconv.Itoa(i/len(prefixes)) + ":" + strconv.Itoa(i)
	}
	return keys
}

func BenchmarkBuildKeyTree2M(b *testing.B) {
	keys := benchKeys(2_000_000)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		dt := NewDataTree("root")
		for _, k := range keys {
			dt.AddKey(k)
		}
	}
}

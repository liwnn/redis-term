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

// BenchmarkAddKey-4   	21802245	        54.9 ns/op	       0 B/op	       0 allocs/op
// BenchmarkAddKey-4   	   54043	    119964 ns/op	     138 B/op	       2 allocs/op
func BenchmarkAddKey(b *testing.B) {
	tree := NewDataTree("root")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tree.AddKey("a:b:" + strconv.Itoa(i))
	}
}

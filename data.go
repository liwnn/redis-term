package redisterm

import (
	"fmt"
	"io"
	"log"
	"redisterm/redis"
	"strconv"
	"strings"
)

// Data data
type Data struct {
	redis *Redis

	db    []*DataTree
	index int
}

// NewData new
func NewData(redis *Redis) *Data {
	r := &Data{
		redis: redis,
	}

	return r
}

// GetDatabases database name
func (d *Data) GetDatabases() []*DataNode {
	if len(d.db) == 0 {
		dbNum, err := d.redis.GetDatabases()
		if err != nil {
			return nil
		}
		for index := 0; index < dbNum; index++ {
			n := NewDataTree("db" + strconv.Itoa(index))
			d.db = append(d.db, n)
		}
	}

	r := make([]*DataNode, 0, len(d.db))
	for _, v := range d.db {
		r = append(r, v.root)
	}
	return r
}

// Cmd cmd
func (d *Data) Cmd(w io.Writer, cmd string) error {
	args := strings.Fields(cmd)
	r, err := d.redis.client.Do(args...)
	if err != nil {
		return err
	}

	switch r.Type() {
	case redis.Int:
		v, err := r.Int()
		if err != nil {
			return err
		}
		fmt.Fprint(w, v)
	case redis.Err, redis.BulkStr, redis.SimpleStr:
		fmt.Fprint(w, r.String())
	case redis.Array:
		l, _ := r.List()
		for _, v := range l {
			fmt.Fprintln(w, v)
		}
	default:
		fmt.Fprintf(w, "cmd no implement %v", r.Type())
	}
	return nil
}

// ScanAllKeys get all key
func (d *Data) ScanAllKeys() []*DataNode {
	n := d.db[d.index]

	var cursor = "0"
	for {
		var keys []string
		cursor, keys = d.redis.Scan(cursor, "*", 10000)
		for _, key := range keys {
			n.AddKey(key)
		}
		if cursor == "0" {
			break
		}
	}

	return n.GetChildren(n.root)
}

// GetKeys get key
func (d *Data) GetKeys() []*DataNode {
	keys := d.redis.Keys("*")
	n := d.db[d.index]
	for _, key := range keys {
		n.AddKey(key)
	}

	return n.GetChildren(n.root)
}

// GetChildren child
func (d *Data) GetChildren(node *DataNode) []*DataNode {
	return node.child
}

// Select select db
func (d *Data) Select(index int) {
	if index == d.index {
		return
	}
	d.redis.Select(index)
	d.index = index
}

// Rename key -> newKey
func (d *Data) Rename(node *DataNode, newKey string) {
	err := d.redis.Rename(node.key, newKey)
	if err != nil {
		log.Fatal(err)
	}
	node.key = newKey
	index := strings.LastIndex(newKey, ":")
	if index != -1 {
		node.name = newKey[index+1:]
	} else {
		node.name = newKey
	}
}

// GetValue value
func (d *Data) GetValue(key string) interface{} {
	val := d.redis.Type(key)
	switch val {
	case "string":
		b, err := d.redis.GetByte(key)
		if err != nil {
			return nil
		}
		if isText(b) {
			return string(b)
		}
		return encodeToHexString(b)
	case "hash":
		return d.redis.GetKV(key)
	case "set":
		return d.redis.GetSet(key)
	case "none":
		return nil
	default:
		return fmt.Sprintf("%v not implement!!!", val)
	}
}

// Delete node
func (d *Data) Delete(node *DataNode) {
	d.redis.Del(node.key)
	node.removed = true
	for _, v := range node.GetChildren() {
		d.Delete(v)
	}
}

// FlushDB remove all keys from current database.
func (d *Data) FlushDB(node *DataNode) {
	d.redis.FlushDB()
	node.ClearChildren()
}

// Reload reload.
func (d *Data) Reload(node *DataNode) {
	Log("Data: Reload key %v*", node.key)
	node.ClearChildren()

	var cursor = "0"
	for {
		var keys []string
		cursor, keys = d.redis.Scan(cursor, node.key+"*", 10000)
		for _, key := range keys {
			d.db[d.index].AddKey(key)
		}
		if cursor == "0" {
			break
		}
	}

	if !node.HasChild() {
		node.RemoveSelf()
		node.removed = true
	}
}

// Close close
func (d *Data) Close() {
	d.redis.Close()
}

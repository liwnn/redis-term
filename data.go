package redisterm

import (
	"fmt"
	"strconv"
)

// Data data
type Data struct {
	redis *Redis

	db []*DataTree
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

// GetKeys get key
func (d *Data) GetKeys(index int) []*DataNode {
	d.redis.Select(index)
	keys := d.redis.Keys("*")
	n := d.db[index]
	for _, key := range keys {
		n.AddKey(key)
	}

	return n.GetChildren(n.root)
}

// GetChildren child
func (d *Data) GetChildren(node *DataNode) []*DataNode {
	return node.child
}

// GetValue value
func (d *Data) GetValue(index int, key string) interface{} {
	d.redis.Select(index)
	val := d.redis.Type(key)
	switch val {
	case "string":
		b := d.redis.GetByte(key)
		if isText(b) {
			return (string(b))
		}
		return encodeToHexString(b)
	case "hash":
		return d.redis.GetKV(key)
	case "set":
		return d.redis.GetSet(key)
	default:
		return fmt.Sprintf("%v not implement!!!", val)
	}
}

// Delete node
func (d *Data) Delete(node *DataNode) {
	d.redis.Del(node.key)
	node.removed = true
}

package redisterm

import (
	"errors"
	"fmt"
	"io"
	"log"
	"strconv"
	"strings"

	"redisterm/redis"
	"redisterm/tlog"
)

var (
	ErrDBNotConnect = errors.New("Db not connect")
)

// Data data
type Data struct {
	redis   *Redis
	address string
	auth    string

	db    []*DataTree
	index int
}

// NewData new
func NewData(addr string, auth string) *Data {
	r := &Data{
		address: addr,
		auth:    auth,
	}

	return r
}

// Connect db
func (d *Data) Connect() error {
	client, err := NewRedis(d.address, d.auth)
	if err != nil {
		return err
	}
	d.redis = client
	return nil
}

// GetDatabases database name
func (d *Data) GetDatabases() ([]*DataNode, error) {
	if d.redis == nil {
		return nil, ErrDBNotConnect
	}
	if len(d.db) == 0 {
		dbNum, err := d.redis.GetDatabases()
		if err != nil {
			return nil, err
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
	return r, nil
}

// Cmd cmd
func (d *Data) Cmd(w io.Writer, cmd string) error {
	if d.redis == nil {
		return nil
	}
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
		fmt.Fprintln(w, r.String())
	case redis.Array:
		l, _ := r.List()
		for _, v := range l {
			fmt.Fprintln(w, v)
		}
	default:
		fmt.Fprintf(w, "cmd no implement %v\n", r.Type())
	}
	return nil
}

// ScanAllKeys get all key
func (d *Data) ScanAllKeys() ([]*DataNode, error) {
	if d.redis == nil {
		return nil, ErrDBNotConnect
	}
	n := d.db[d.index]

	var cursor = "0"
	for {
		var keys []string
		var err error
		cursor, keys, err = d.redis.Scan(cursor, "*", 10000)
		if err != nil {
			return nil, err
		}
		for _, key := range keys {
			n.AddKey(key)
		}
		if cursor == "0" {
			break
		}
	}

	return n.GetChildren(n.root), nil
}

// GetKeys get key
func (d *Data) GetKeys() []*DataNode {
	if d.redis == nil {
		return nil
	}
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
func (d *Data) Select(index int) error {
	if d.redis == nil {
		return ErrDBNotConnect
	}
	if index == d.index {
		return nil
	}
	if err := d.redis.Select(index); err != nil {
		return err
	}

	d.index = index
	return nil
}

// Rename key -> newKey
func (d *Data) Rename(node *DataNode, newKey string) {
	if d.redis == nil {
		return
	}
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
	if d.redis == nil {
		return nil
	}
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
	case "list":
		return d.redis.GetList(key)
	case "none":
		return nil
	default:
		return fmt.Sprintf("%v not implement!!!", val)
	}
}

func (d *Data) SetValue(node *DataNode, value string) error {
	if d.redis == nil {
		return ErrDBNotConnect
	}
	err := d.redis.Set(node.key, value)
	if err != nil {
		return err
	}
	return nil
}

// Delete node
func (d *Data) Delete(node *DataNode) error {
	if d.redis == nil {
		return ErrDBNotConnect
	}
	if err := d.redis.Del(node.key); err != nil {
		return err
	}
	node.removed = true
	for _, v := range node.GetChildren() {
		d.Delete(v)
	}
	return nil
}

// FlushDB remove all keys from current database.
func (d *Data) FlushDB(node *DataNode) error {
	if d.redis == nil {
		return ErrDBNotConnect
	}
	if err := d.redis.FlushDB(); err != nil {
		return err
	}
	node.ClearChildren()
	return nil
}

// Reload reload.
func (d *Data) Reload(node *DataNode) error {
	if d.redis == nil {
		return nil
	}
	tlog.Log("Data: Reload key %v*", node.key)
	node.ClearChildren()

	var cursor = "0"
	for {
		var keys []string
		var err error
		cursor, keys, err = d.redis.Scan(cursor, node.key+"*", 10000)
		if err != nil {
			return err
		}
		for _, key := range keys {
			d.db[d.index].AddKey(key)
		}
		if cursor == "0" {
			break
		}
	}

	if !node.HasChild() {
		node.RemoveSelf()
	}
	return nil
}

// Close close
func (d *Data) Close() {
	if d.redis == nil {
		return
	}
	d.redis.Close()
}

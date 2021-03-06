package redisterm

import (
	"errors"
	"net"
	"strconv"

	"redisterm/redis"
)

// KVText kv
type KVText struct {
	Key   string
	Value string
}

// Redis client
type Redis struct {
	client *redis.Client
}

// NewRedis new
func NewRedis(address string, auth string) (*Redis, error) {
	conn, err := net.Dial("tcp", address)
	if err != nil {
		return nil, err
	}
	client := redis.NewClient(conn)
	if len(auth) > 0 {
		r, err := client.Do("AUTH", auth)
		if err != nil {
			return nil, err
		}
		Log("AUTH %v", r.String())
	}
	return &Redis{
		client: client,
	}, nil
}

// Close close conn.
func (r *Redis) Close() {
	r.client.Close()
}

// GetDatabases return database count.
func (r *Redis) GetDatabases() (int, error) {
	result, err := r.client.Do("config", "get", "databases")
	if err != nil {
		return 0, err
	}
	d, err := result.List()
	if err != nil {
		return 0, err
	}

	Log("[Redis] config get databases")
	return strconv.Atoi(d[1])
}

// Scan the keys
func (r *Redis) Scan(cursor string, match string, count int) (string, []string, error) {
	countStr := strconv.Itoa(count)
	result, err := r.client.Do("SCAN", cursor, "MATCH", match, "COUNT", countStr)
	if err != nil {
		return "", nil, err
	}
	if result == nil {
		return "", nil, err
	}
	d := result.ToArray()
	if len(d) != 2 {
		return "", nil, err
	}
	nextCursor := d[0].String()
	keys, _ := d[1].List()

	Log("[Redis] scan %v MATCH %v COUNT %v", cursor, match, count)
	return nextCursor, keys, nil
}

// Keys keys
func (r *Redis) Keys(pattern string) []string {
	result, err := r.client.Do("keys", pattern)
	if err != nil {
		return nil
	}
	d, err := result.List()
	if err != nil {
		return nil
	}
	Log("[Redis] keys %v", pattern)
	return d
}

// Type type
func (r *Redis) Type(key string) string {
	result, err := r.client.Do("type", key)
	if err != nil {
		return ""
	}
	Log("[Redis] type %v", key)
	return result.String()
}

// Get get
func (r *Redis) Get(key string) string {
	result, err := r.client.Do("GET", key)
	if err != nil {
		return ""
	}

	Log("[Redis] get %v", key)
	return result.String()
}

// GetByte get
func (r *Redis) GetByte(key string) ([]byte, error) {
	result, err := r.client.Do("GET", key)
	if err != nil {
		return nil, err
	}

	Log("[Redis] get %v", key)
	if result.IsNil() {
		return nil, errors.New("nil")
	}

	return result.Byte(), nil
}

// GetKV hash
func (r *Redis) GetKV(key string) []KVText {
	result, err := r.client.Do("HGETAll", key)
	if err != nil {
		return nil
	}

	elems, err := result.List()
	if err != nil {
		return nil
	}
	h := make([]KVText, 0, len(elems)/2)
	for i := 0; i < len(elems)/2; i++ {
		h = append(h, KVText{elems[i*2], elems[i*2+1]})
	}
	Log("[Redis] get %v", key)
	return h
}

// GetSet set members
func (r *Redis) GetSet(key string) []string {
	result, err := r.client.Do("SMEMBERS", key)
	if err != nil {
		return nil
	}

	elems, err := result.List()
	if err != nil {
		return nil
	}
	Log("[Redis] get %v", key)
	return elems
}

// GetList return list members.
func (r *Redis) GetList(key string) []string {
	result, err := r.client.Do("lrange", key, "0", "-1")
	if err != nil {
		return nil
	}

	elems, err := result.List()
	if err != nil {
		return nil
	}
	Log("[Redis] get %v", key)
	return elems
}

// Select select index
func (r *Redis) Select(index int) error {
	result, err := r.client.Do("SELECT", strconv.Itoa(index))
	if err != nil {
		return err
	}
	if result.String() != "OK" {
		return errors.New(result.String())
	}
	Log("[Redis] select %v", index)
	return nil
}

// Rename key -> newKey
func (r *Redis) Rename(key, newKey string) error {
	result, err := r.client.Do("RENAME", key, newKey)
	if err != nil {
		return err
	}
	Log("[Redis] rename %v -> %v, resp[%v]", key, newKey, result.String())
	return nil
}

// Del delete a key.
func (r *Redis) Del(key string) error {
	result, err := r.client.Do("DEL", key)
	if err != nil {
		return err
	}

	Log("[Redis] DEL %v %v", key, result)
	return nil
}

// FlushDB remove all keys from current database.
func (r *Redis) FlushDB() error {
	result, err := r.client.Do("FLUSHDB")
	if err != nil {
		return err
	}

	Log("[Redis] FLUSHDB  %v", result)
	return nil
}

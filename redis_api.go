package redisterm

import (
	"log"
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
	index  int
}

// NewRedis new
func NewRedis(address string) *Redis {
	conn, err := net.Dial("tcp", address)
	if err != nil {
		log.Fatalln(err)
	}
	client := redis.NewClient(conn)
	return &Redis{
		client: client,
	}
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

	Log("Redis: config get databases")
	return strconv.Atoi(d[1])
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
	Log("Redis: keys %v", pattern)
	return d
}

// Type type
func (r *Redis) Type(key string) string {
	result, err := r.client.Do("type", key)
	if err != nil {
		return ""
	}
	Log("Redis: type %v", key)
	return result.String()
}

// Get get
func (r *Redis) Get(key string) string {
	result, err := r.client.Do("GET", key)
	if err != nil {
		return ""
	}

	Log("Redis: get %v", key)
	return result.String()
}

// GetByte get
func (r *Redis) GetByte(key string) ([]byte, error) {
	result, err := r.client.Do("GET", key)
	if err != nil {
		return nil, err
	}

	Log("Redis: get %v", key)
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
	Log("Redis: get %v", key)
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
	Log("Redis: get %v", key)
	return elems
}

// Select select index
func (r *Redis) Select(index int) {
	if index == r.index {
		return
	}
	result, err := r.client.Do("SELECT", strconv.Itoa(index))
	if err != nil {
		log.Fatalln(err)
	}
	if result.String() != "OK" {
		log.Fatalln(result.String())
	}
	r.index = index

	Log("Redis: select %v", index)
}

// Del delete a key.
func (r *Redis) Del(key string) {
	result, err := r.client.Do("DEL", key)
	if err != nil {
		log.Fatalln(err)
	}

	Log("Redis: DEL %v %v", key, result)
}

// FlushDB remove all keys from current database.
func (r *Redis) FlushDB() {
	result, err := r.client.Do("FLUSHDB")
	if err != nil {
		log.Fatalln(err)
	}

	Log("Redis: FLUSHDB index[%v] -  %v", r.index, result)
}

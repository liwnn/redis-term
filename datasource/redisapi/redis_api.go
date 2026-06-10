package redisapi

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"time"

	"github.com/liwnn/redisterm/datasource/redis"
	"github.com/liwnn/redisterm/tlog"
)

// RedisConfig is a connection config. Kind selects the backend
// ("redis" or empty for redis, "mongo" for mongo). Redis uses
// host/port/auth; mongo uses uri.
type RedisConfig struct {
	Name string `json:"name"`
	Host string `json:"host"`
	Port int    `json:"port"`
	Auth string `json:"auth"`
	Kind string `json:"kind,omitempty"`
	URI  string `json:"uri,omitempty"`  // mongo connection string (URL mode)
	User string `json:"user,omitempty"` // mysql / mongo username (form mode)
	DB   string `json:"db,omitempty"`   // mongo default database (form mode)
}

// MongoURI resolves the mongo connection string. When URI is set the connection
// was entered in URL mode and is used verbatim; otherwise it is assembled from
// the host/port/user/auth/db form fields.
func (c RedisConfig) MongoURI() string {
	if c.URI != "" {
		return c.URI
	}
	host := c.Host
	if host == "" {
		host = "127.0.0.1"
	}
	port := c.Port
	if port == 0 {
		port = 27017
	}
	var auth string
	if c.User != "" {
		auth = url.QueryEscape(c.User)
		if c.Auth != "" {
			auth += ":" + url.QueryEscape(c.Auth)
		}
		auth += "@"
	}
	return fmt.Sprintf("mongodb://%s%s:%d/%s", auth, host, port, c.DB)
}

// KVText kv
type KVText struct {
	Key   string
	Value string
}

type ZSetText KVText

// StreamEntry is one stream entry with its ID and ordered field/value pairs,
// kept structured so each field can become its own table column.
type StreamEntry struct {
	ID     string
	Fields []KVText
}

// Redis client
type Redis struct {
	client *redis.Client
}

// NewRedis new
func NewRedis(address string, auth string) (*Redis, error) {
	// Dial with a timeout so an unreachable host fails fast instead of freezing
	// the UI thread until the OS-level TCP timeout (which can be minutes).
	conn, err := net.DialTimeout("tcp", address, 5*time.Second)
	if err != nil {
		return nil, err
	}
	client := redis.NewClient(conn)
	if len(auth) > 0 {
		r, err := client.Do("AUTH", auth)
		if err != nil {
			return nil, err
		}
		tlog.Log("[Redis] AUTH %v", r.String())
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

	tlog.Log("[Redis] config get databases")
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

	tlog.Log("[Redis] scan %v MATCH %v COUNT %v", cursor, match, count)
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
	tlog.Log("[Redis] keys %v", pattern)
	return d
}

// Type type
func (r *Redis) Type(key string) string {
	result, err := r.client.Do("type", key)
	if err != nil {
		return ""
	}
	tlog.Log("[Redis] type %v", key)
	return result.String()
}

// Get get
func (r *Redis) Get(key string) string {
	result, err := r.client.Do("GET", key)
	if err != nil {
		return ""
	}

	tlog.Log("[Redis] GET %v", key)
	return result.String()
}

// GetByte get
func (r *Redis) GetByte(key string) ([]byte, error) {
	result, err := r.client.Do("GET", key)
	if err != nil {
		return nil, err
	}

	tlog.Log("[Redis] get %v", key)
	if result.IsNil() {
		return nil, errors.New("nil")
	}

	return result.Byte(), nil
}

// GetHash hash
func (r *Redis) GetHash(key string) []KVText {
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
	tlog.Log("[Redis] HGETAll %v", key)
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
	tlog.Log("[Redis] SMEMBERS %v", key)
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
	tlog.Log("[Redis] lrange %v", key)
	return elems
}

func (r *Redis) Do(cmd string, params ...string) (*redis.Reply, error) {
	tlog.Log("[Redis] cmd[%v] params[%v]", cmd, params)
	return r.client.Do(cmd, params...)
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
	tlog.Log("[Redis] select %v", index)
	return nil
}

// Rename key -> newKey
func (r *Redis) Rename(key, newKey string) error {
	result, err := r.client.Do("RENAME", key, newKey)
	if err != nil {
		return err
	}
	tlog.Log("[Redis] rename %v -> %v, resp[%v]", key, newKey, result.String())
	return nil
}

// Set key -> value
func (r *Redis) Set(key, value string) error {
	result, err := r.client.Do("SET", key, value)
	if err != nil {
		return err
	}
	tlog.Log("[Redis] set %v -> %v, resp[%v]", key, value, result.String())
	return nil
}

// Del delete a key.
func (r *Redis) Del(key string) error {
	result, err := r.client.Do("DEL", key)
	if err != nil {
		return err
	}

	tlog.Log("[Redis] DEL %v %v", key, result)
	return nil
}

// FlushDB remove all keys from current database.
func (r *Redis) FlushDB() error {
	result, err := r.client.Do("FLUSHDB")
	if err != nil {
		return err
	}

	tlog.Log("[Redis] FLUSHDB  %v", result)
	return nil
}

func (r *Redis) ZRange(key string, start, stop int) []ZSetText {
	result, err := r.client.Do("ZRANGE", key, strconv.Itoa(start), strconv.Itoa(stop), "WITHSCORES")
	if err != nil {
		tlog.Log("[Redis] ZRange %v", key)
		return nil
	}
	elems, err := result.List()
	if err != nil {
		tlog.Log("[Redis] ZRange %v", key)
		return nil
	}
	h := make([]ZSetText, 0, len(elems)/2)
	for i := 0; i < len(elems)/2; i++ {
		h = append(h, ZSetText{elems[i*2], elems[i*2+1]})
	}
	tlog.Log("[Redis] ZRANGE %v %v %v", key, start, stop)
	return h
}

// GetStreamEntries returns stream entries via XRANGE with field/value pairs kept
// structured, so each distinct field can be rendered as its own column.
func (r *Redis) GetStreamEntries(key string) []StreamEntry {
	result, err := r.client.Do("XRANGE", key, "-", "+")
	if err != nil {
		tlog.Log("[Redis] XRANGE %v err %v", key, err)
		return nil
	}
	entries := result.ToArray()
	h := make([]StreamEntry, 0, len(entries))
	for _, entry := range entries {
		pair := entry.ToArray()
		if len(pair) < 2 {
			continue
		}
		id := pair[0].String()
		fields, err := pair[1].List()
		var kvs []KVText
		if err == nil {
			for i := 0; i+1 < len(fields); i += 2 {
				kvs = append(kvs, KVText{Key: fields[i], Value: fields[i+1]})
			}
		}
		h = append(h, StreamEntry{ID: id, Fields: kvs})
	}
	tlog.Log("[Redis] XRANGE %v", key)
	return h
}

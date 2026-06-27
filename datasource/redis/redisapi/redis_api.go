package redisapi

import (
	"errors"
	"net"
	"strconv"
	"time"

	"github.com/liwnn/redisterm/datasource/redis/redisapi/resp"
)

// Logger receives debug/trace lines (format + args, like fmt.Printf). It lets a
// host app route this client's logging wherever it wants without this package
// depending on a concrete logger. A nil Logger disables logging entirely.
type Logger interface {
	Log(format string, args ...any)
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
	client *resp.Client
	logger Logger
}

// logf logs through the injected Logger, if any.
func (r *Redis) logf(format string, args ...any) {
	if r.logger != nil {
		r.logger.Log(format, args...)
	}
}

// NewRedis dials address and returns a client. logger may be nil to disable
// this client's debug logging.
func NewRedis(address string, auth string, logger Logger) (*Redis, error) {
	// Dial with a timeout so an unreachable host fails fast instead of freezing
	// the UI thread until the OS-level TCP timeout (which can be minutes).
	conn, err := net.DialTimeout("tcp", address, 5*time.Second)
	if err != nil {
		return nil, err
	}
	r := &Redis{
		client: resp.NewClient(conn),
		logger: logger,
	}
	if len(auth) > 0 {
		s, err := r.client.DoStatus("AUTH", auth)
		if err != nil {
			return nil, err
		}
		r.logf("[Redis] AUTH %v", s)
	}
	return r, nil
}

// Close close conn.
func (r *Redis) Close() {
	r.client.Close()
}

// GetDatabases return database count.
func (r *Redis) GetDatabases() (int, error) {
	d, err := r.client.DoStringArray("config", "get", "databases")
	if err != nil {
		return 0, err
	}
	if len(d) < 2 {
		return 0, errors.New("unexpected CONFIG GET databases reply")
	}
	r.logf("[Redis] config get databases")
	return strconv.Atoi(d[1])
}

// DBSize returns the number of keys in the current db.
func (r *Redis) DBSize() (int, error) {
	value, err := r.client.DoInt("DBSIZE")
	if err != nil {
		return 0, err
	}
	r.logf("[Redis] dbsize %v", value)
	return value, nil
}

// Scan the keys
func (r *Redis) Scan(cursor string, match string, count int) (string, []string, error) {
	countStr := strconv.Itoa(count)
	nextCursor, keys, err := r.client.DoScan("SCAN", cursor, "MATCH", match, "COUNT", countStr)
	if err != nil {
		return "", nil, err
	}
	r.logf("[Redis] scan %v MATCH %v COUNT %v", cursor, match, count)
	return nextCursor, keys, nil
}

// Keys keys
func (r *Redis) Keys(pattern string) []string {
	d, err := r.client.DoStringArray("keys", pattern)
	if err != nil {
		return nil
	}
	r.logf("[Redis] keys %v", pattern)
	return d
}

// Type type
func (r *Redis) Type(key string) string {
	t, err := r.client.DoStatus("type", key)
	if err != nil {
		return ""
	}
	r.logf("[Redis] type %v", key)
	return t
}

// Get get
func (r *Redis) Get(key string) string {
	b, err := r.client.DoBulk("GET", key)
	if err != nil {
		return ""
	}
	r.logf("[Redis] GET %v", key)
	return string(b)
}

// GetByte get
func (r *Redis) GetByte(key string) ([]byte, error) {
	b, err := r.client.DoBulk("GET", key)
	if err != nil {
		return nil, err
	}
	r.logf("[Redis] get %v", key)
	if b == nil {
		return nil, errors.New("nil")
	}
	return b, nil
}

// GetHash hash
func (r *Redis) GetHash(key string) []KVText {
	elems, err := r.client.DoStringArray("HGETAll", key)
	if err != nil {
		return nil
	}
	h := make([]KVText, 0, len(elems)/2)
	for i := 0; i < len(elems)/2; i++ {
		h = append(h, KVText{elems[i*2], elems[i*2+1]})
	}
	r.logf("[Redis] HGETAll %v", key)
	return h
}

// GetSet set members
func (r *Redis) GetSet(key string) []string {
	elems, err := r.client.DoStringArray("SMEMBERS", key)
	if err != nil {
		return nil
	}
	r.logf("[Redis] SMEMBERS %v", key)
	return elems
}

// GetList return list members.
func (r *Redis) GetList(key string) []string {
	elems, err := r.client.DoStringArray("lrange", key, "0", "-1")
	if err != nil {
		return nil
	}
	r.logf("[Redis] lrange %v", key)
	return elems
}

// Do runs an arbitrary command and returns its reply as a Go-native value
// (string / int / []any / nil), for callers (redis-cli) that can't know the
// reply type in advance.
func (r *Redis) Do(cmd string, params ...string) (any, error) {
	r.logf("[Redis] cmd[%v] params[%v]", cmd, params)
	return r.client.DoReply(cmd, params...)
}

// Ping verifies the connection with a PING command. A non-PONG status or any
// transport error means the connection is down.
func (r *Redis) Ping() error {
	s, err := r.client.DoStatus("PING")
	if err != nil {
		return err
	}
	if s != "PONG" {
		return errors.New(s)
	}
	return nil
}

// Select select index
func (r *Redis) Select(index int) error {
	s, err := r.client.DoStatus("SELECT", strconv.Itoa(index))
	if err != nil {
		return err
	}
	if s != "OK" {
		return errors.New(s)
	}
	r.logf("[Redis] select %v", index)
	return nil
}

// Rename key -> newKey
func (r *Redis) Rename(key, newKey string) error {
	s, err := r.client.DoStatus("RENAME", key, newKey)
	if err != nil {
		return err
	}
	r.logf("[Redis] rename %v -> %v, resp[%v]", key, newKey, s)
	return nil
}

// Set key -> value
func (r *Redis) Set(key, value string) error {
	s, err := r.client.DoStatus("SET", key, value)
	if err != nil {
		return err
	}
	r.logf("[Redis] set %v -> %v, resp[%v]", key, value, s)
	return nil
}

// Del delete a key.
func (r *Redis) Del(key string) error {
	n, err := r.client.DoInt("DEL", key)
	if err != nil {
		return err
	}
	r.logf("[Redis] DEL %v %v", key, n)
	return nil
}

// FlushDB remove all keys from current database.
func (r *Redis) FlushDB() error {
	s, err := r.client.DoStatus("FLUSHDB")
	if err != nil {
		return err
	}
	r.logf("[Redis] FLUSHDB  %v", s)
	return nil
}

func (r *Redis) ZRange(key string, start, stop int) []ZSetText {
	elems, err := r.client.DoStringArray("ZRANGE", key, strconv.Itoa(start), strconv.Itoa(stop), "WITHSCORES")
	if err != nil {
		r.logf("[Redis] ZRange %v", key)
		return nil
	}
	h := make([]ZSetText, 0, len(elems)/2)
	for i := 0; i < len(elems)/2; i++ {
		h = append(h, ZSetText{elems[i*2], elems[i*2+1]})
	}
	r.logf("[Redis] ZRANGE %v %v %v", key, start, stop)
	return h
}

// GetStreamEntries returns stream entries via XRANGE with field/value pairs kept
// structured, so each distinct field can be rendered as its own column.
func (r *Redis) GetStreamEntries(key string) []StreamEntry {
	rows, err := r.client.DoStream("XRANGE", key, "-", "+")
	if err != nil {
		r.logf("[Redis] XRANGE %v err %v", key, err)
		return nil
	}
	h := make([]StreamEntry, 0, len(rows))
	for _, row := range rows {
		// row is [id, f1, v1, f2, v2, ...]
		if len(row) == 0 {
			continue
		}
		id := row[0]
		fields := row[1:]
		var kvs []KVText
		for i := 0; i+1 < len(fields); i += 2 {
			kvs = append(kvs, KVText{Key: fields[i], Value: fields[i+1]})
		}
		h = append(h, StreamEntry{ID: id, Fields: kvs})
	}
	r.logf("[Redis] XRANGE %v", key)
	return h
}

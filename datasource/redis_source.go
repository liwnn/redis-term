package datasource

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/liwnn/redisterm/datasource/redis"
	"github.com/liwnn/redisterm/datasource/redisapi"
)

// RedisSource adapts a redis connection to the Datasource interface.
// Containers are named "db0".."dbN"; entries are keys.
type RedisSource struct {
	address string
	auth    string
	redis   *redisapi.Redis
}

var _ Datasource = (*RedisSource)(nil)

// NewRedisSource new
func NewRedisSource(address, auth string) *RedisSource {
	return &RedisSource{address: address, auth: auth}
}

func (s *RedisSource) Open() error {
	r, err := redisapi.NewRedis(s.address, s.auth)
	if err != nil {
		return err
	}
	s.redis = r
	return nil
}

func (s *RedisSource) Close() {
	if s.redis != nil {
		s.redis.Close()
	}
}

// dbIndex parses "db3" -> 3.
func dbIndex(container string) (int, error) {
	n := strings.TrimPrefix(container, "db")
	return strconv.Atoi(n)
}

// selectDB switches to the db named by container.
func (s *RedisSource) selectDB(container string) error {
	idx, err := dbIndex(container)
	if err != nil {
		return fmt.Errorf("invalid container %q", container)
	}
	return s.redis.Select(idx)
}

func (s *RedisSource) Containers() ([]string, error) {
	n, err := s.redis.GetDatabases()
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, n)
	for i := 0; i < n; i++ {
		names = append(names, "db"+strconv.Itoa(i))
	}
	return names, nil
}

func (s *RedisSource) Entries(container string) ([]string, error) {
	if err := s.selectDB(container); err != nil {
		return nil, err
	}
	var keys []string
	cursor := "0"
	for {
		next, batch, err := s.redis.Scan(cursor, "*", 1000)
		if err != nil {
			return nil, err
		}
		keys = append(keys, batch...)
		cursor = next
		if cursor == "0" {
			break
		}
	}
	return keys, nil
}

func (s *RedisSource) Content(container, entry string, _ Page) (Content, error) {
	if err := s.selectDB(container); err != nil {
		return Content{}, err
	}
	t := s.redis.Type(entry)
	c := Content{Type: t}
	switch t {
	case "string":
		b, err := s.redis.GetByte(entry)
		if err != nil {
			c.Kind = KindText
			c.Text = ""
			return c, nil
		}
		c.Kind = KindText
		c.Text = string(b)
		c.Total = 1
	case "hash":
		h := s.redis.GetHash(entry)
		c.Kind = KindTable
		c.Columns = []string{"key", "value"}
		c.Rows = make([][]string, 0, len(h))
		for _, kv := range h {
			c.Rows = append(c.Rows, []string{kv.Key, kv.Value})
		}
		c.Total = len(h)
	case "zset":
		z := s.redis.ZRange(entry, 0, -1)
		c.Kind = KindTable
		c.Columns = []string{"member", "score"}
		c.Rows = make([][]string, 0, len(z))
		for _, kv := range z {
			c.Rows = append(c.Rows, []string{kv.Key, kv.Value})
		}
		c.Total = len(z)
	case "set":
		members := s.redis.GetSet(entry)
		c.Kind = KindTable
		c.Columns = []string{"value"}
		c.Rows = make([][]string, 0, len(members))
		for _, v := range members {
			c.Rows = append(c.Rows, []string{v})
		}
		c.Total = len(members)
	case "list":
		items := s.redis.GetList(entry)
		c.Kind = KindTable
		c.Columns = []string{"value"}
		c.Rows = make([][]string, 0, len(items))
		for _, v := range items {
			c.Rows = append(c.Rows, []string{v})
		}
		c.Total = len(items)
	case "stream":
		entries := s.redis.GetStreamEntries(entry)
		c.Kind = KindTable
		// Columns are the entry ID plus the union of all field names, in first-seen
		// order, so heterogeneous stream entries still line up by field.
		fieldCols := make([]string, 0)
		colIdx := make(map[string]int)
		for _, e := range entries {
			for _, kv := range e.Fields {
				if _, ok := colIdx[kv.Key]; !ok {
					colIdx[kv.Key] = len(fieldCols)
					fieldCols = append(fieldCols, kv.Key)
				}
			}
		}
		c.Columns = append([]string{"id"}, fieldCols...)
		c.Rows = make([][]string, 0, len(entries))
		for _, e := range entries {
			row := make([]string, len(c.Columns))
			row[0] = e.ID
			for _, kv := range e.Fields {
				row[colIdx[kv.Key]+1] = kv.Value
			}
			c.Rows = append(c.Rows, row)
		}
		c.Total = len(entries)
	case "none":
		return Content{}, fmt.Errorf("key not found")
	default:
		c.Kind = KindText
		c.Text = fmt.Sprintf("%v not implemented", t)
	}
	return c, nil
}

// Update writes a single table cell change back to redis, dispatching by the
// entry's type. OldRow holds the row's original cells (field/value, member/score, ...).
func (s *RedisSource) Update(container, entry string, e Edit) error {
	if err := s.selectDB(container); err != nil {
		return err
	}
	switch t := s.redis.Type(entry); t {
	case "hash":
		// columns: [key(field), value]
		if len(e.OldRow) < 2 {
			return fmt.Errorf("invalid hash row")
		}
		field, oldVal := e.OldRow[0], e.OldRow[1]
		if e.Column == 0 { // rename field
			if e.Value == field {
				return nil
			}
			if err := s.do("HDEL", entry, field); err != nil {
				return err
			}
			return s.do("HSET", entry, e.Value, oldVal)
		}
		return s.do("HSET", entry, field, e.Value)
	case "zset":
		// columns: [member, score]
		if len(e.OldRow) < 2 {
			return fmt.Errorf("invalid zset row")
		}
		member, score := e.OldRow[0], e.OldRow[1]
		if e.Column == 1 { // change score
			return s.do("ZADD", entry, e.Value, member)
		}
		// rename member: add new with same score, remove old
		if e.Value == member {
			return nil
		}
		if err := s.do("ZADD", entry, score, e.Value); err != nil {
			return err
		}
		return s.do("ZREM", entry, member)
	case "list":
		return s.do("LSET", entry, strconv.Itoa(e.Row), e.Value)
	case "set":
		if len(e.OldRow) < 1 {
			return fmt.Errorf("invalid set row")
		}
		old := e.OldRow[0]
		if e.Value == old {
			return nil
		}
		if err := s.do("SREM", entry, old); err != nil {
			return err
		}
		return s.do("SADD", entry, e.Value)
	case "string":
		return s.redis.Set(entry, e.Value)
	default:
		return fmt.Errorf("edit not supported for type %q", t)
	}
}

// DropEntry deletes a key from the db named by container.
func (s *RedisSource) DropEntry(container, entry string) error {
	if err := s.selectDB(container); err != nil {
		return err
	}
	return s.do("DEL", entry)
}

// DropContainer flushes all keys in the db named by container.
func (s *RedisSource) DropContainer(container string) error {
	if err := s.selectDB(container); err != nil {
		return err
	}
	return s.do("FLUSHDB")
}

// do runs a redis command and turns an error reply into a Go error.
func (s *RedisSource) do(cmd string, args ...string) error {
	reply, err := s.redis.Do(cmd, args...)
	if err != nil {
		return err
	}
	if reply.Type() == redis.Err {
		return fmt.Errorf("%s", reply.String())
	}
	return nil
}

func (s *RedisSource) Command(container, raw string) (string, error) {
	if err := s.selectDB(container); err != nil {
		return "", err
	}
	args := strings.Fields(raw)
	if len(args) == 0 {
		return "", fmt.Errorf("empty command")
	}
	reply, err := s.redis.Do(args[0], args[1:]...)
	if err != nil {
		return "", err
	}
	return replyText(reply), nil
}

// replyText renders a redis reply as plain text lines.
func replyText(r *redis.Reply) string {
	switch r.Type() {
	case redis.Nil:
		return "(nil)"
	case redis.Int:
		v, _ := r.Int()
		return strconv.Itoa(v)
	case redis.Array:
		l, _ := r.List()
		return strings.Join(l, "\n")
	default:
		return r.String()
	}
}

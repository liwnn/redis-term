package redisterm

import (
	"fmt"
	"log"
	"strconv"
)

// Data data
type Data struct {
	redis *Redis
}

// NewData new
func NewData(redis *Redis) *Data {
	return &Data{
		redis: redis,
	}
}

// GetDatabases database name
func (d *Data) GetDatabases() []string {
	dbNum, err := d.redis.GetDatabases()
	if err != nil {
		log.Fatalln(err)
	}

	r := make([]string, 0, dbNum)
	for index := 0; index < dbNum; index++ {
		r = append(r, "db"+strconv.Itoa(index))
	}
	return r
}

// GetKeys get key
func (d *Data) GetKeys(index int) []string {
	d.redis.Select(index)
	keys := d.redis.Keys("*")
	return keys
}

// GetValue value
func (d *Data) GetValue(index int, key string) string {
	d.redis.Select(index)
	val := d.redis.Type(key)
	switch val {
	case "string":
		b := d.redis.GetByte(key)
		if isText(b) {
			return (string(b))
		}
		return encodeToHexString(b)
	default:
		return fmt.Sprintf("%v not implement!!!", val)
	}
}

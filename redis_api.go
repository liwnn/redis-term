package redisterm

import (
	"log"
	"net"
	"strconv"

	"redisterm/redis"
)

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

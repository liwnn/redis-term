package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/liwnn/redisterm/datasource/redisapi"
)

// file is the on-disk JSON shape: a connections list plus the name of the
// connection selected last run, so the next launch can restore it.
type file struct {
	Connections  []redisapi.RedisConfig `json:"connections"`
	LastSelected string                 `json:"lastSelected,omitempty"`
}

type Config struct {
	filename     string
	configs      []redisapi.RedisConfig
	lastSelected string // name of the last-selected connection
}

func NewConfig(filename string) (*Config, error) {
	if filename[0] == '~' {
		filename = os.Getenv("HOME") + filename[1:]
	}

	c := &Config{
		filename: filename,
		configs: []redisapi.RedisConfig{
			{
				Name: "127.0.0.1:6379",
				Host: "127.0.0.1",
				Port: 6379,
				Auth: "",
			},
		},
	}
	if _, err := os.Stat(c.filename); err == nil || os.IsExist(err) {
		if err := c.load(); err != nil {
			return nil, err
		}
	}
	return c, nil
}

func (c *Config) load() error {
	b, err := os.ReadFile(c.filename)
	if err != nil {
		return err
	}

	// Current format is an object {connections, lastSelected}; older files are a
	// bare []RedisConfig array. Detect the array by its leading '[' and read it
	// for backward compatibility.
	if trimmed := strings.TrimLeft(string(b), " \t\r\n"); strings.HasPrefix(trimmed, "[") {
		var configs []redisapi.RedisConfig
		if err := json.Unmarshal(b, &configs); err != nil {
			return err
		}
		c.configs = configs
		return nil
	}

	var f file
	if err := json.Unmarshal(b, &f); err != nil {
		return err
	}
	c.configs = f.Connections
	c.lastSelected = f.LastSelected
	return nil
}

func (c *Config) Save() error {
	f := file{
		Connections:  c.configs,
		LastSelected: c.lastSelected,
	}
	b, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(c.filename, b, 0666)
}

func (c *Config) GetConfig(index int) redisapi.RedisConfig {
	return c.configs[index]
}

// SaveLastSelected records the name of the connection at index as the
// last-selected one and persists it to the config file. Write errors are
// ignored: failing to remember the selection just means the next launch falls
// back to the first connection.
func (c *Config) SaveLastSelected(index int) {
	if index < 0 || index >= len(c.configs) {
		return
	}
	c.lastSelected = c.configs[index].Name
	_ = c.Save()
}

// LastSelectedIndex returns the index of the connection that was selected last
// run, resolved by name. It returns 0 when there is no saved state or the saved
// connection no longer exists.
func (c *Config) LastSelectedIndex() int {
	if c.lastSelected == "" {
		return 0
	}
	for i, v := range c.configs {
		if v.Name == c.lastSelected {
			return i
		}
	}
	return 0
}

// kindColor maps a backend kind to a tview color tag used to tint its dropdown
// entry so redis/mongo/mysql are visually distinct.
func kindColor(kind string) string {
	switch kind {
	case "mongo":
		return "#8bc99a" // light green
	case "mysql":
		return "#90caf9" // light blue
	default: // redis
		return "#f6b8b1" // lighter red
	}
}

// GetDbNames returns dropdown labels with tview color tags: a kind-colored,
// kind-labeled prefix followed by the connection name, so redis/mongo/mysql are
// distinguishable by both color and text. The consumer (OpLine) must have style
// tags enabled. The KIND label is wrapped in full-width brackets (not ASCII
// [ ]), since ASCII brackets would be parsed as (invalid) tview tags.
func (c *Config) GetDbNames() []string {
	var s = make([]string, 0, len(c.configs))
	for _, v := range c.configs {
		kind := v.Kind
		if kind == "" {
			kind = "redis"
		}
		s = append(s, fmt.Sprintf("[%s]【%s】[-]%s", kindColor(kind), strings.ToUpper(kind), v.Name))
	}
	return s
}

// Count returns the number of configured connections.
func (c *Config) Count() int {
	return len(c.configs)
}

func (c *Config) Update(conf redisapi.RedisConfig, index int) {
	if index < 0 || index >= len(c.configs) {
		c.Add(conf)
		return
	}
	c.configs[index] = conf
}

func (c *Config) Add(conf redisapi.RedisConfig) {
	c.configs = append(c.configs, conf)
}

// Remove deletes the connection at index. Out-of-range indices are ignored.
func (c *Config) Remove(index int) {
	if index < 0 || index >= len(c.configs) {
		return
	}
	c.configs = append(c.configs[:index], c.configs[index+1:]...)
}

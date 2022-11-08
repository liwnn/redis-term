package config

import (
	"encoding/json"
	"os"

	"github.com/liwnn/redisterm/redisapi"
)

type Config struct {
	filename string
	configs  []redisapi.RedisConfig
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

	var configs []redisapi.RedisConfig
	if err := json.Unmarshal(b, &configs); err != nil {
		return err
	}
	c.configs = configs
	return nil
}

func (c *Config) Save() error {
	b, err := json.MarshalIndent(c.configs, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(c.filename, b, 0666)
}

func (c *Config) GetConfig(index int) redisapi.RedisConfig {
	return c.configs[index]
}

func (c *Config) GetDbNames() []string {
	var s = make([]string, 0, len(c.configs))
	for _, v := range c.configs {
		s = append(s, v.Name)
	}
	return s
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

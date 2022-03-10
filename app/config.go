package app

import (
	"encoding/json"
	"io/ioutil"
	"redisterm/redisapi"
)

type config struct {
	filename string
	configs  []redisapi.RedisConfig
}

func newConfig(filename string) *config {
	c := &config{
		filename: filename,
	}
	if err := c.load(); err != nil {
		panic(err)
	}
	return c
}

func (c *config) load() error {
	b, err := ioutil.ReadFile(c.filename)
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

func (c *config) save() error {
	b, err := json.MarshalIndent(c.configs, "", "  ")
	if err != nil {
		return err
	}
	return ioutil.WriteFile(c.filename, b, 0)
}

func (c *config) getConfig(index int) redisapi.RedisConfig {
	return c.configs[index]
}

func (c *config) getDbNames() []string {
	var s = make([]string, 0, len(c.configs))
	for _, v := range c.configs {
		s = append(s, v.Name)
	}
	return s
}

func (c *config) update(conf redisapi.RedisConfig) bool {
	for i, v := range c.configs {
		if v.Host == conf.Host && v.Port == conf.Port {
			c.configs[i] = conf
			return false
		}
	}
	c.configs = append(c.configs, conf)
	return true
}

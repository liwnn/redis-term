package main

import (
	"encoding/json"
	"flag"
	"io/ioutil"
	"redisterm"
)

var config string

func init() {
	flag.StringVar(&config, "config", "./config.json", "config")
}

func main() {
	flag.Parse()

	b, err := ioutil.ReadFile(config)
	if err != nil {
		panic(err)
	}

	v := make([]redisterm.RedisConfig, 0)
	if err := json.Unmarshal(b, &v); err != nil {
		panic(err)
	}

	app := redisterm.NewApp()
	app.Run(v...)
}

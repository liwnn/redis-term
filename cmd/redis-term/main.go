package main

import (
	"flag"
	"redisterm"
)

var config string

func init() {
	flag.StringVar(&config, "config", "./config.json", "config")
}

func main() {
	flag.Parse()

	app := redisterm.NewApp(config)
	app.Run()
}

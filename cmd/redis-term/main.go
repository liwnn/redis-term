package main

import (
	"flag"

	"redisterm/app"
)

var config string

func init() {
	flag.StringVar(&config, "config", "./config.json", "config")
}

func main() {
	flag.Parse()

	app.NewApp(config).Run()
}

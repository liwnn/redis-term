package main

import (
	"flag"

	"github.com/liwnn/redisterm/app"
)

var config string

func init() {
	flag.StringVar(&config, "config", "~/.redis-term.json", "config")
}

func main() {
	flag.Parse()

	app.NewApp(config).Run()
}

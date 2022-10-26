package main

import (
	"flag"
	"os"

	"github.com/liwnn/redisterm/app"
)

var config string

func init() {
	flag.StringVar(&config, "config", "~/.redis-term.json", "config")
}

func main() {
	flag.Parse()

	if config[0] == '~' {
		config = os.Getenv("HOME") + config[1:]
	}

	app.NewApp(config).Run()
}

package main

import (
	"flag"

	"github.com/liwnn/redisterm/tui/app"
)

var configFile string

func init() {
	flag.StringVar(&configFile, "config", "~/.redis-term.json", "config")
}

func main() {
	flag.Parse()

	app.NewApp(configFile).Run()
}

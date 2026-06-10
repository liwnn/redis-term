package main

import (
	"flag"

	"github.com/liwnn/redisterm/config"
	"github.com/liwnn/redisterm/web"
)

var (
	configFile string
	addr       string
)

func init() {
	flag.StringVar(&configFile, "config", "~/.redis-term.json", "config")
	flag.StringVar(&addr, "addr", ":8080", "web UI listen address")
}

func main() {
	flag.Parse()

	cfg, err := config.NewConfig(configFile)
	if err != nil {
		panic(err)
	}
	if err := web.NewServer(cfg).Run(addr); err != nil {
		panic(err)
	}
}

package main

import (
	"flag"
	"redisterm"
)

var (
	host string
	port int
	auth string
)

func init() {
	flag.StringVar(&host, "h", "127.0.0.1", "hostname(default:127.0.0.1)")
	flag.IntVar(&port, "p", 6379, "port(default:6379)")
	flag.StringVar(&auth, "a", "", "auth")
}

func main() {
	flag.Parse()

	var configs = []redisterm.RedisConfig{
		{
			Name: "127.0.0.1:9898",
			Host: host,
			Port: port,
			Auth: auth,
		},
		{
			Name: "172.25.128.116:9898",
			Host: "172.25.128.116",
			Port: 9898,
			Auth: "sanguo",
		},
	}

	app := redisterm.NewApp()
	app.Run(configs...)
}

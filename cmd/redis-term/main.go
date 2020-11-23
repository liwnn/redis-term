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

	redisterm.Run(host, port, auth)
}

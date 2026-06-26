package config

import (
	"fmt"
	"net/url"
)

// Conn is a single connection's configuration. Kind selects the backend
// ("redis" or empty for redis, "mongo"/"mysql"/"zookeeper" otherwise). Redis
// uses host/port/auth; mongo uses uri (URL mode) or the host/port/user/auth/db
// form fields.
type Conn struct {
	Name string `json:"name"`
	Host string `json:"host"`
	Port int    `json:"port"`
	Auth string `json:"auth"`
	Kind string `json:"kind,omitempty"`
	URI  string `json:"uri,omitempty"`  // mongo connection string (URL mode)
	User string `json:"user,omitempty"` // mysql / mongo username (form mode)
	DB   string `json:"db,omitempty"`   // mongo default database (form mode)
}

// MongoURI resolves the mongo connection string. When URI is set the connection
// was entered in URL mode and is used verbatim; otherwise it is assembled from
// the host/port/user/auth/db form fields.
func (c Conn) MongoURI() string {
	if c.URI != "" {
		return c.URI
	}
	host := c.Host
	if host == "" {
		host = "127.0.0.1"
	}
	port := c.Port
	if port == 0 {
		port = 27017
	}
	var auth string
	if c.User != "" {
		auth = url.QueryEscape(c.User)
		if c.Auth != "" {
			auth += ":" + url.QueryEscape(c.Auth)
		}
		auth += "@"
	}
	return fmt.Sprintf("mongodb://%s%s:%d/%s", auth, host, port, c.DB)
}

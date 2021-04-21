package redisterm

import (
	"io"
	"log"
	"os"
)

var (
	global = log.New(os.Stderr, "", log.LstdFlags)
)

// SetLogger set output
func SetLogger(logger io.Writer) {
	global.SetOutput(logger)
}

// Log log
func Log(format string, params ...interface{}) {
	global.Printf(format, params...)
}

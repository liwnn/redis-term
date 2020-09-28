package redisterm

import (
	"fmt"
	"io"
	"os"
)

var (
	global io.Writer = os.Stdout
)

// SetLogger set output
func SetLogger(logger io.Writer) {
	global = logger
}

// Log log
func Log(format string, params ...interface{}) {
	fmt.Fprintf(global, format, params...)
	fmt.Fprintln(global)
}

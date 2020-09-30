package redisterm

import "fmt"

func Log(format string, params ...interface{}) {
	fmt.Fprintf(outputText, format, params...)
	fmt.Fprintln(outputText)
}

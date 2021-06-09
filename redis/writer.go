package redis

import (
	"bufio"
	"io"
	"strconv"
)

var (
	arrayPrefixSlice      = []byte{'*'}
	bulkStringPrefixSlice = []byte{'$'}
	lineEndingSlice       = []byte{'\r', '\n'}
)

// RESPWriter w
type RESPWriter struct {
	*bufio.Writer
}

// NewRESPWriter new
func NewRESPWriter(writer io.Writer) *RESPWriter {
	return &RESPWriter{
		Writer: bufio.NewWriter(writer),
	}
}

// WriteCommand write
// @param args - All Redis commands are sent as arrays of bulk strings. *3\r\n$3\r\nSET\r\n$5\r\nmykey\r\n$8\r\nmy value\r\n
func (w *RESPWriter) WriteCommand(args ...string) (err error) {
	if _, err := w.Write(arrayPrefixSlice); err != nil {
		return err
	}
	if _, err := w.WriteString(strconv.Itoa(len(args))); err != nil {
		return err
	}
	if _, err := w.Write(lineEndingSlice); err != nil {
		return err
	}

	for _, arg := range args {
		w.Write(bulkStringPrefixSlice)
		w.WriteString(strconv.Itoa(len(arg)))
		w.Write(lineEndingSlice)
		w.WriteString(arg)
		w.Write(lineEndingSlice)
	}

	return w.Flush()
}

package redis

// EncodeToHexString renders binary bytes as \xHH escape sequences.
func EncodeToHexString(src []byte) string {
	const hextable = "0123456789ABCDEF"
	dst := make([]byte, len(src)*4)
	j := 0
	for _, v := range src {
		dst[j] = '\\'
		dst[j+1] = 'x'
		dst[j+2] = hextable[v>>4]
		dst[j+3] = hextable[v&0x0f]
		j += 4
	}
	return string(dst)
}

// IsText reports whether b looks like text rather than binary. Empty is text; a
// NUL byte means binary; more than 30% non-printable bytes means binary.
func IsText(b []byte) bool {
	if len(b) == 0 {
		return true
	}
	var count int
	for _, v := range b {
		if v == 0 {
			return false
		}
		if v >= 32 && v <= 126 || (v == '\n' || v == '\r' || v == '\t' || v == '\b') {
			continue
		}
		count++
	}
	isBin := count*100 >= len(b)*30
	return !isBin
}

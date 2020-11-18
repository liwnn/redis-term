package redisterm

func encodeToHexString(src []byte) string {
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

func isText(b []byte) bool {
	var count int
	for _, v := range b {
		if v == 0 { // '\0' 则不是文本
			return false
		}
		if v>>7 == 1 {
			count++
		}
	}
	return count*100 >= len(b)*30
}

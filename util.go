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
	// 空字符串按照文本格式处理
	if len(b) == 0 {
		return true
	}
	var count int
	for _, v := range b {
		// 如果字符串含有空字符（‘\0’），则认为是二进制格式
		if v == 0 {
			return false
		}
		if v>>7 == 1 {
			count++
		}
	}
	// 超过30%的字符串高位时1（ascii大于126）或其它奇怪字符，则认为是二进制格式
	isBin := count*100 >= len(b)*30
	return !isBin
}

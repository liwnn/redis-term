package resp

// This file holds command-specific reply decoders: unlike the pure RESP
// primitives in reader.go (which only know the five wire types), these know the
// shape of a particular command's reply (SCAN, XRANGE) and assemble it from the
// primitives. Keeping them out of reader.go keeps the protocol layer free of
// command knowledge.

// readScanReply decodes a SCAN reply: a 2-element array of [cursor, keys-array],
// straight into the cursor string and the key slice.
func (r *respReader) readScanReply() (cursor string, keys []string, err error) {
	n, err := r.readArrayHeader()
	if err != nil {
		return "", nil, err
	}
	if n != 2 {
		return "", nil, ErrInvalidSyntax
	}
	cur, err := r.readBulk()
	if err != nil {
		return "", nil, err
	}
	cursor = string(cur) // small; copied so it doesn't pin the read buffer
	keys, err = r.readStringArray()
	if err != nil {
		return "", nil, err
	}
	return cursor, keys, nil
}

// readStreamReply decodes an XRANGE reply (array of [id, [f,v,f,v,...]]) into one
// flat []string per entry: [id, f1, v1, f2, v2, ...]. Returning a backend-neutral
// shape keeps this package free of the redisapi StreamEntry type.
func (r *respReader) readStreamReply() ([][]string, error) {
	n, err := r.readArrayHeader()
	if err != nil {
		return nil, err
	}
	if n <= 0 {
		return nil, nil
	}
	out := make([][]string, 0, n)
	for i := 0; i < n; i++ {
		// each entry is a 2-element array: [id(bulk), fields(array)]
		m, err := r.readArrayHeader()
		if err != nil {
			return nil, err
		}
		if m != 2 {
			return nil, ErrInvalidSyntax
		}
		idb, err := r.readBulk()
		if err != nil {
			return nil, err
		}
		fields, err := r.readStringArray()
		if err != nil {
			return nil, err
		}
		row := make([]string, 0, 1+len(fields))
		row = append(row, string(idb))
		row = append(row, fields...)
		out = append(out, row)
	}
	return out, nil
}

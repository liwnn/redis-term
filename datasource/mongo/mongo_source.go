package mongo

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/liwnn/redisterm/datasource"
)

// defaultDocLimit caps how many documents a collection preview loads.
const defaultDocLimit = 100

// MongoSource adapts a mongo connection to the Datasource interface.
// Containers are databases; entries are collections.
type MongoSource struct {
	uri    string
	client *mongo.Client
}

var (
	_ datasource.Datasource = (*MongoSource)(nil)
	_ datasource.Pinger     = (*MongoSource)(nil)
)

// NewMongoSource builds a source for a mongodb URI (e.g. "mongodb://host:27017").
func NewMongoSource(uri string) *MongoSource {
	return &MongoSource{uri: uri}
}

func ctx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 10*time.Second)
}

func (s *MongoSource) Open() error {
	c, cancel := ctx()
	defer cancel()
	client, err := mongo.Connect(options.Client().ApplyURI(s.uri))
	if err != nil {
		return err
	}
	if err := client.Ping(c, nil); err != nil {
		return err
	}
	s.client = client
	return nil
}

// Ping verifies the connection is alive, so the UI can show a health dot. The
// mongo driver pools and auto-reconnects, so unlike redis this reuses the main
// client rather than a dedicated connection.
func (s *MongoSource) Ping() error {
	if s.client == nil {
		return fmt.Errorf("not connected")
	}
	c, cancel := ctx()
	defer cancel()
	return s.client.Ping(c, nil)
}

func (s *MongoSource) Close() {
	if s.client != nil {
		c, cancel := ctx()
		defer cancel()
		_ = s.client.Disconnect(c)
	}
}

func (s *MongoSource) Containers() ([]string, error) {
	c, cancel := ctx()
	defer cancel()
	return s.client.ListDatabaseNames(c, bson.M{})
}

func (s *MongoSource) Entries(container string) ([]string, error) {
	c, cancel := ctx()
	defer cancel()
	names, err := s.client.Database(container).ListCollectionNames(c, bson.M{})
	if err != nil {
		return nil, err
	}
	sort.Strings(names)
	return names, nil
}

func (s *MongoSource) Content(container, entry string, page datasource.Page) (datasource.Content, error) {
	c, cancel := ctx()
	defer cancel()
	coll := s.client.Database(container).Collection(entry)

	limit := int64(page.Limit)
	if limit <= 0 {
		limit = defaultDocLimit
	}
	// A .limit(N) chained on the query statement overrides the paging default,
	// so editing the displayed statement actually changes how many docs load.
	if n, ok := shellLimit(page.Query); ok {
		limit = n
	}
	opt := options.Find().SetLimit(limit)
	if page.Skip > 0 {
		opt.SetSkip(int64(page.Skip))
	}

	filter, err := parseQuery(page.Query)
	if err != nil {
		return datasource.Content{}, err
	}

	cur, err := coll.Find(c, filter, opt)
	if err != nil {
		return datasource.Content{}, err
	}
	defer cur.Close(c)

	var docs []bson.M
	if err := cur.All(c, &docs); err != nil {
		return datasource.Content{}, err
	}

	total, err := coll.CountDocuments(c, filter)
	if err != nil {
		total = int64(len(docs))
	}

	columns := columnUnion(docs)
	rows := make([][]string, 0, len(docs))
	cellTypes := make([][]string, 0, len(docs))
	for _, d := range docs {
		row := make([]string, len(columns))
		types := make([]string, len(columns))
		for i, col := range columns {
			if v, ok := d[col]; ok {
				row[i] = valueString(v)
				types[i] = valueType(v)
			} else {
				types[i] = "missing"
			}
		}
		rows = append(rows, row)
		cellTypes = append(cellTypes, types)
	}

	return datasource.Content{
		Kind:      datasource.KindTable,
		Type:      "collection",
		Columns:   columns,
		Rows:      rows,
		CellTypes: cellTypes,
		Total:     int(total),
		Query:     findStatement(entry, filter, limit),
	}, nil
}

// findStatement renders the shell-style find() call that produced the content,
// e.g. db.users.find({"age":{"$gt":18}}).limit(100). An empty filter renders as
// find({}); the limit reflects the applied document cap.
func findStatement(entry string, filter interface{}, limit int64) string {
	body := "{}"
	if b, err := bson.MarshalExtJSON(filter, false, false); err == nil {
		body = string(b)
	}
	return fmt.Sprintf("db.%s.find(%s).limit(%d)", entry, body, limit)
}

// Update writes a single document field change via UpdateOne matched on _id.
// The new value is coerced to e.OldType so numeric/bool fields keep their type.
// Rename is not supported for mongo.
func (s *MongoSource) Rename(_, _, _ string) error {
	return fmt.Errorf("rename not supported")
}

func (s *MongoSource) Update(container, entry string, e datasource.Edit) error {
	if e.Column < 0 || e.Column >= len(e.Columns) {
		return fmt.Errorf("invalid column")
	}
	field := e.Columns[e.Column]
	if field == "_id" {
		return fmt.Errorf("cannot edit _id")
	}

	idVal, err := docID(e.Columns, e.OldRow, e.OldRowTypes)
	if err != nil {
		return err
	}
	val, err := coerce(e.Value, e.OldType)
	if err != nil {
		return err
	}

	c, cancel := ctx()
	defer cancel()
	coll := s.client.Database(container).Collection(entry)
	res, err := coll.UpdateOne(c, bson.M{"_id": idVal}, bson.M{"$set": bson.M{field: val}})
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return fmt.Errorf("no document matched _id")
	}
	return nil
}

// docID extracts the _id value from a row and coerces it to its real bson
// type so the UpdateOne query matches non-ObjectID keys (int64, string, ...).
func docID(columns, row, rowTypes []string) (interface{}, error) {
	idx := -1
	for i, c := range columns {
		if c == "_id" {
			idx = i
			break
		}
	}
	if idx < 0 || idx >= len(row) {
		return nil, fmt.Errorf("row has no _id")
	}
	raw := row[idx]
	idType := ""
	if idx < len(rowTypes) {
		idType = rowTypes[idx]
	}
	switch idType {
	case "objectId":
		oid, err := bson.ObjectIDFromHex(raw)
		if err != nil {
			return nil, fmt.Errorf("invalid _id: %v", err)
		}
		return oid, nil
	case "int", "double", "bool":
		return coerce(raw, idType)
	case "":
		// type unknown (older callers): best-effort ObjectID, else string.
		if oid, err := bson.ObjectIDFromHex(raw); err == nil {
			return oid, nil
		}
		return raw, nil
	default:
		return raw, nil
	}
}

// coerce converts the edited string to the original bson type.
func coerce(value, oldType string) (interface{}, error) {
	switch oldType {
	case "int":
		n, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("expected int: %v", err)
		}
		return n, nil
	case "double":
		f, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return nil, fmt.Errorf("expected number: %v", err)
		}
		return f, nil
	case "bool":
		b, err := strconv.ParseBool(value)
		if err != nil {
			return nil, fmt.Errorf("expected bool: %v", err)
		}
		return b, nil
	case "object":
		var m bson.M
		if err := json.Unmarshal([]byte(value), &m); err != nil {
			return nil, fmt.Errorf("expected JSON object: %v", err)
		}
		return m, nil
	case "array":
		var a bson.A
		if err := json.Unmarshal([]byte(value), &a); err != nil {
			return nil, fmt.Errorf("expected JSON array: %v", err)
		}
		return a, nil
	case "null":
		if value == "" || value == "null" {
			return nil, nil
		}
		return value, nil
	default: // string, date, objectId-on-non-id, missing, etc.
		return value, nil
	}
}

// parseQuery turns a user-typed filter into a bson document. An empty string
// matches all. Two forms are accepted:
//   - a bare filter document: {"field": value}
//   - mongo-shell style: db.<collection>.find({...}) — the first argument
//     (the filter) is extracted; any projection/options are ignored.
//
// The filter is parsed as MongoDB Extended JSON so operators and typed values
// ($gt, $oid, dates, ...) work as in the mongo shell.
func parseQuery(query string) (interface{}, error) {
	q := strings.TrimSpace(query)
	if q == "" {
		return bson.M{}, nil
	}
	if f, ok := shellFilter(q); ok {
		q = f
	}
	if q == "" {
		return bson.M{}, nil
	}
	var filter bson.M
	if err := bson.UnmarshalExtJSON([]byte(q), false, &filter); err != nil {
		return nil, fmt.Errorf("invalid query: %v", err)
	}
	return filter, nil
}

// shellFilter extracts the filter argument from a mongo-shell find() call such
// as `db.activity.find({"a":1})` or `db.coll.find({...}, {...})`. It returns
// the first argument and true when the input looks like a find() call; the
// second return is false when the input is not shell-style (treat as raw JSON).
func shellFilter(q string) (string, bool) {
	open := strings.Index(q, ".find(")
	if !strings.HasPrefix(q, "db.") || open < 0 {
		return "", false
	}
	start := open + len(".find(")
	// Find the paren that closes this find(...) call, not the last ')' in the
	// string — a trailing chain like .limit(100) has its own parens. Track depth
	// across (), skipping {} / [] contents so a paren inside the filter doesn't
	// throw off the count.
	end := -1
	depth, brace := 1, 0
	for i := start; i < len(q); i++ {
		switch q[i] {
		case '{', '[':
			brace++
		case '}', ']':
			brace--
		case '(':
			if brace == 0 {
				depth++
			}
		case ')':
			if brace == 0 {
				depth--
				if depth == 0 {
					end = i
				}
			}
		}
		if end >= 0 {
			break
		}
	}
	if end < start {
		return "", false
	}
	inner := strings.TrimSpace(q[start:end])
	if inner == "" {
		return "", true // db.coll.find() => match all
	}
	// Take the first argument (the filter), splitting on the top-level comma
	// that separates filter from projection.
	depth = 0
	for i, r := range inner {
		switch r {
		case '{', '[':
			depth++
		case '}', ']':
			depth--
		case ',':
			if depth == 0 {
				return strings.TrimSpace(inner[:i]), true
			}
		}
	}
	return inner, true
}

// shellLimit extracts N from a `.limit(N)` chained on a find() statement, e.g.
// db.users.find({}).limit(50). Returns the value and true when a valid positive
// limit is present; false when absent or unparseable (caller keeps its default).
func shellLimit(q string) (int64, bool) {
	i := strings.LastIndex(q, ".limit(")
	if i < 0 {
		return 0, false
	}
	rest := q[i+len(".limit("):]
	end := strings.IndexByte(rest, ')')
	if end < 0 {
		return 0, false
	}
	n, err := strconv.ParseInt(strings.TrimSpace(rest[:end]), 10, 64)
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}

// DeleteRows removes documents matched precisely by their _id (coerced to its
// real bson type), one DeleteOne per row — never a bulk match.
func (s *MongoSource) DeleteRows(container, entry string, columns []string, rows []datasource.RowRef) error {
	c, cancel := ctx()
	defer cancel()
	coll := s.client.Database(container).Collection(entry)
	for _, r := range rows {
		idVal, err := docID(columns, r.Row, r.Types)
		if err != nil {
			return err
		}
		res, err := coll.DeleteOne(c, bson.M{"_id": idVal})
		if err != nil {
			return err
		}
		if res.DeletedCount == 0 {
			return fmt.Errorf("no document matched _id")
		}
	}
	return nil
}

// DropEntry drops a collection from a database.
func (s *MongoSource) DropEntry(container, entry string) error {
	c, cancel := ctx()
	defer cancel()
	return s.client.Database(container).Collection(entry).Drop(c)
}

// DropContainer drops an entire database.
func (s *MongoSource) DropContainer(container string) error {
	c, cancel := ctx()
	defer cancel()
	return s.client.Database(container).Drop(c)
}

// Command runs a mongo-shell style statement against the selected database
// (container). Supported forms:
//
//	db.<coll>.find(<filter>, <projection>)
//	db.<coll>.findOne(<filter>, <projection>)
//	db.<coll>.countDocuments(<filter>)               (alias: count)
//	db.<coll>.insertOne(<doc>)
//	db.<coll>.insertMany([<doc>, ...])
//	db.<coll>.updateOne(<filter>, <update>)
//	db.<coll>.updateMany(<filter>, <update>)
//	db.<coll>.deleteOne(<filter>)
//	db.<coll>.deleteMany(<filter>)
//	db.runCommand(<command>)
//
// All arguments are parsed as MongoDB Extended JSON, so typed values and
// operators ($set, $oid, dates, ...) work as in the mongo shell.
func (s *MongoSource) Command(container, raw string) (string, error) {
	// Tolerate a trailing semicolon (mongo-shell habit): strip it so the
	// `)`-suffix checks in parseShellCall/trimCall still match.
	q := strings.TrimRight(strings.TrimSpace(raw), ";")
	q = strings.TrimSpace(q)
	c, cancel := ctx()
	defer cancel()

	// mongo-shell helpers (no `db.` prefix). `show dbs` needs no selected
	// database, so handle these before the container guard below.
	switch strings.ToLower(q) {
	case "show dbs", "show databases":
		names, err := s.client.ListDatabaseNames(c, bson.M{})
		if err != nil {
			return "", err
		}
		return strings.Join(names, "\n"), nil
	case "show collections", "show tables":
		if container == "" {
			return "", fmt.Errorf("select a database first")
		}
		names, err := s.Entries(container)
		if err != nil {
			return "", err
		}
		return strings.Join(names, "\n"), nil
	case "db":
		if container == "" {
			return "", fmt.Errorf("select a database first")
		}
		return container, nil
	}

	if container == "" {
		return "", fmt.Errorf("select a database first")
	}
	db := s.client.Database(container)

	if arg, ok := trimCall(q, "db.runCommand("); ok {
		var cmd bson.D
		if err := bson.UnmarshalExtJSON([]byte(arg), false, &cmd); err != nil {
			return "", fmt.Errorf("invalid command: %v", err)
		}
		var res bson.M
		if err := db.RunCommand(c, cmd).Decode(&res); err != nil {
			return "", err
		}
		return marshalExt(res)
	}

	coll, method, args, ok := parseShellCall(q)
	if !ok {
		return "", fmt.Errorf("unsupported command; try db.<coll>.find({...}) or db.runCommand({...})")
	}
	collection := db.Collection(coll)
	switch method {
	case "find":
		filter, projection, err := parseFindArgs(args)
		if err != nil {
			return "", err
		}
		opt := options.Find().SetLimit(defaultDocLimit)
		if projection != nil {
			opt.SetProjection(projection)
		}
		cur, err := collection.Find(c, filter, opt)
		if err != nil {
			return "", err
		}
		defer cur.Close(c)
		var docs []bson.M
		if err := cur.All(c, &docs); err != nil {
			return "", err
		}
		return marshalDocs(docs)
	case "findOne":
		filter, projection, err := parseFindArgs(args)
		if err != nil {
			return "", err
		}
		opt := options.FindOne()
		if projection != nil {
			opt.SetProjection(projection)
		}
		var doc bson.M
		if err := collection.FindOne(c, filter, opt).Decode(&doc); err != nil {
			if err == mongo.ErrNoDocuments {
				return "null", nil
			}
			return "", err
		}
		return marshalExt(doc)
	case "count", "countDocuments":
		filter, _, err := parseFindArgs(args)
		if err != nil {
			return "", err
		}
		n, err := collection.CountDocuments(c, filter)
		if err != nil {
			return "", err
		}
		return strconv.FormatInt(n, 10), nil
	case "insertOne":
		var doc bson.D
		if err := bson.UnmarshalExtJSON([]byte(args), false, &doc); err != nil {
			return "", fmt.Errorf("invalid document: %v", err)
		}
		res, err := collection.InsertOne(c, doc)
		if err != nil {
			return "", err
		}
		return marshalExt(bson.M{"acknowledged": true, "insertedId": res.InsertedID})
	case "insertMany":
		var docs []interface{}
		if err := bson.UnmarshalExtJSON([]byte(args), false, &docs); err != nil {
			return "", fmt.Errorf("invalid documents: %v", err)
		}
		res, err := collection.InsertMany(c, docs)
		if err != nil {
			return "", err
		}
		return marshalExt(bson.M{"acknowledged": true, "insertedIds": res.InsertedIDs})
	case "updateOne", "updateMany":
		filter, update, err := parseUpdateArgs(args)
		if err != nil {
			return "", err
		}
		var res *mongo.UpdateResult
		if method == "updateOne" {
			res, err = collection.UpdateOne(c, filter, update)
		} else {
			res, err = collection.UpdateMany(c, filter, update)
		}
		if err != nil {
			return "", err
		}
		return marshalExt(bson.M{
			"acknowledged":  true,
			"matchedCount":  res.MatchedCount,
			"modifiedCount": res.ModifiedCount,
			"upsertedCount": res.UpsertedCount,
		})
	case "deleteOne", "deleteMany":
		filter, _, err := parseFindArgs(args)
		if err != nil {
			return "", err
		}
		var res *mongo.DeleteResult
		if method == "deleteOne" {
			res, err = collection.DeleteOne(c, filter)
		} else {
			res, err = collection.DeleteMany(c, filter)
		}
		if err != nil {
			return "", err
		}
		return marshalExt(bson.M{"acknowledged": true, "deletedCount": res.DeletedCount})
	default:
		return "", fmt.Errorf("unsupported method %q", method)
	}
}

// parseUpdateArgs parses the filter (arg 0) and update document (arg 1) of an
// updateOne()/updateMany() call. Both are required.
func parseUpdateArgs(args string) (filter, update bson.M, err error) {
	parts := splitTopLevelCommas(args)
	if len(parts) < 2 {
		return nil, nil, fmt.Errorf("update requires a filter and an update document")
	}
	filter = bson.M{}
	if f := strings.TrimSpace(parts[0]); f != "" {
		if err := bson.UnmarshalExtJSON([]byte(f), false, &filter); err != nil {
			return nil, nil, fmt.Errorf("invalid filter: %v", err)
		}
	}
	update = bson.M{}
	if u := strings.TrimSpace(parts[1]); u != "" {
		if err := bson.UnmarshalExtJSON([]byte(u), false, &update); err != nil {
			return nil, nil, fmt.Errorf("invalid update: %v", err)
		}
	}
	return filter, update, nil
}

// trimCall returns the argument inside a `prefix...)` call when q matches that
// exact shape (e.g. trimCall("db.runCommand({ping:1})", "db.runCommand(")).
func trimCall(q, prefix string) (string, bool) {
	if !strings.HasPrefix(q, prefix) || !strings.HasSuffix(q, ")") {
		return "", false
	}
	return strings.TrimSpace(q[len(prefix) : len(q)-1]), true
}

// parseShellCall splits a `db.<coll>.<method>(<args>)` call into its parts. The
// collection name may itself contain dots; the method is the segment right
// before the opening paren.
func parseShellCall(q string) (coll, method, args string, ok bool) {
	if !strings.HasPrefix(q, "db.") || !strings.HasSuffix(q, ")") {
		return "", "", "", false
	}
	body := q[len("db.") : len(q)-1] // strip leading "db." and trailing ")"
	open := strings.Index(body, "(")
	if open < 0 {
		return "", "", "", false
	}
	path := body[:open]
	dot := strings.LastIndex(path, ".")
	if dot < 0 {
		return "", "", "", false
	}
	coll, method = path[:dot], path[dot+1:]
	if coll == "" || method == "" {
		return "", "", "", false
	}
	return coll, method, strings.TrimSpace(body[open+1:]), true
}

// parseFindArgs parses the filter (arg 0) and optional projection (arg 1) of a
// find()/findOne() call. An empty or missing filter matches everything.
func parseFindArgs(args string) (filter, projection bson.M, err error) {
	filter = bson.M{}
	if args == "" {
		return filter, nil, nil
	}
	parts := splitTopLevelCommas(args)
	if f := strings.TrimSpace(parts[0]); f != "" {
		if err := bson.UnmarshalExtJSON([]byte(f), false, &filter); err != nil {
			return nil, nil, fmt.Errorf("invalid filter: %v", err)
		}
	}
	if len(parts) > 1 {
		if p := strings.TrimSpace(parts[1]); p != "" {
			projection = bson.M{}
			if err := bson.UnmarshalExtJSON([]byte(p), false, &projection); err != nil {
				return nil, nil, fmt.Errorf("invalid projection: %v", err)
			}
		}
	}
	return filter, projection, nil
}

// splitTopLevelCommas splits on commas that sit outside any {} or [] nesting,
// so a filter and projection separate without breaking nested objects.
func splitTopLevelCommas(s string) []string {
	var parts []string
	depth, start := 0, 0
	for i, r := range s {
		switch r {
		case '{', '[':
			depth++
		case '}', ']':
			depth--
		case ',':
			if depth == 0 {
				parts = append(parts, s[start:i])
				start = i + 1
			}
		}
	}
	return append(parts, s[start:])
}

// marshalExt renders a bson value as indented extended JSON so mongo types
// (ObjectID, dates, int64) survive in the command output.
func marshalExt(v interface{}) (string, error) {
	b, err := bson.MarshalExtJSONIndent(v, false, false, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// marshalDocs renders a list of documents as a JSON array. bson's ExtJSON
// marshaller only writes documents at the top level (not a bare array), so each
// doc is marshalled on its own and joined into a bracketed, indented list.
func marshalDocs(docs []bson.M) (string, error) {
	if len(docs) == 0 {
		return "[]", nil
	}
	parts := make([]string, 0, len(docs))
	for _, d := range docs {
		s, err := marshalExt(d)
		if err != nil {
			return "", err
		}
		// indent every line one level so the doc nests under the array bracket
		s = "  " + strings.ReplaceAll(s, "\n", "\n  ")
		parts = append(parts, s)
	}
	return "[\n" + strings.Join(parts, ",\n") + "\n]", nil
}

// columnUnion returns the field names across docs, with _id first and the
// remaining fields in first-seen order so column layout is stable.
func columnUnion(docs []bson.M) []string {
	seen := make(map[string]bool)
	var cols []string
	hasID := false
	for _, d := range docs {
		if _, ok := d["_id"]; ok {
			hasID = true
		}
		for k := range d {
			if k == "_id" || seen[k] {
				continue
			}
			seen[k] = true
			cols = append(cols, k)
		}
	}
	sort.Strings(cols)
	if hasID {
		cols = append([]string{"_id"}, cols...)
	}
	return cols
}

// valueType maps a bson value to a coercion label used on write-back.
func valueType(v interface{}) string {
	switch v.(type) {
	case string:
		return "string"
	case int32, int64:
		return "int"
	case float64:
		return "double"
	case bool:
		return "bool"
	case bson.ObjectID:
		return "objectId"
	case bson.DateTime:
		return "date"
	case bson.M, bson.D:
		return "object"
	case bson.A:
		return "array"
	case nil:
		return "null"
	default:
		return "string"
	}
}

// valueString renders a bson value as a compact display string.
func valueString(v interface{}) string {
	switch x := v.(type) {
	case string:
		return x
	case bson.ObjectID:
		return x.Hex()
	case nil:
		return ""
	case bson.M, bson.A, bson.D:
		b, err := json.Marshal(x)
		if err != nil {
			return fmt.Sprintf("%v", x)
		}
		return string(b)
	default:
		return fmt.Sprintf("%v", x)
	}
}

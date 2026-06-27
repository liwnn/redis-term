package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"github.com/liwnn/redisterm/datasource"
)

// defaultRowLimit caps how many rows a table preview loads.
const defaultRowLimit = 100

// MySQLSource adapts a mysql connection to the Datasource interface.
// Containers are databases; entries are tables.
type MySQLSource struct {
	dsn string
	db  *sql.DB
}

var (
	_ datasource.Datasource = (*MySQLSource)(nil)
	_ datasource.Pinger     = (*MySQLSource)(nil)
)

// NewMySQLSource builds a source from connection parts, assembling a
// go-sql-driver DSN like "user:pass@tcp(127.0.0.1:3306)/".
func NewMySQLSource(host string, port int, user, password string) *MySQLSource {
	auth := user
	if password != "" {
		auth += ":" + password
	}
	// timeout bounds the dial so an unreachable host fails fast instead of
	// freezing the UI thread; read/write timeouts bound subsequent queries.
	dsn := fmt.Sprintf("%s@tcp(%s:%d)/?timeout=5s&readTimeout=10s&writeTimeout=10s", auth, host, port)
	return &MySQLSource{dsn: dsn}
}

func (s *MySQLSource) Open() error {
	db, err := sql.Open("mysql", s.dsn)
	if err != nil {
		return err
	}
	c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(c); err != nil {
		db.Close()
		return err
	}
	s.db = db
	return nil
}

func (s *MySQLSource) Close() {
	if s.db != nil {
		s.db.Close()
	}
}

// Ping verifies the connection is alive, so the UI can show a health dot.
// database/sql pools and auto-reconnects, so this reuses the pool rather than a
// dedicated connection.
func (s *MySQLSource) Ping() error {
	if s.db == nil {
		return fmt.Errorf("not connected")
	}
	c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return s.db.PingContext(c)
}

func (s *MySQLSource) Containers() ([]string, error) {
	names, err := s.queryStrings("SHOW DATABASES")
	if err != nil {
		return nil, err
	}
	sort.Strings(names)
	return names, nil
}

func (s *MySQLSource) Entries(container string) ([]string, error) {
	names, err := s.queryStrings("SHOW TABLES FROM " + quoteIdent(container))
	if err != nil {
		return nil, err
	}
	sort.Strings(names)
	return names, nil
}

func (s *MySQLSource) Content(container, entry string, page datasource.Page) (datasource.Content, error) {
	limit := page.Limit
	if limit <= 0 {
		limit = defaultRowLimit
	}
	table := quoteIdent(container) + "." + quoteIdent(entry)

	// The query box may carry either a full SELECT-shaped statement (the user
	// edited the displayed statement and pressed Enter) or a bare WHERE
	// fragment. A full statement runs verbatim, after a read-only guard so the
	// box can't mutate data; a fragment is folded into a default SELECT.
	var (
		q        string
		where    string
		fullStmt bool
	)
	if trimmed := strings.TrimSpace(page.Query); trimmed != "" && isReadOnlyStmt(trimmed) {
		q = trimmed
		fullStmt = true
	} else {
		where = whereClause(page.Query)
		q = fmt.Sprintf("SELECT * FROM %s%s LIMIT %d", table, where, limit)
		if page.Skip > 0 {
			q += fmt.Sprintf(" OFFSET %d", page.Skip)
		}
	}

	columns, rows, cellTypes, err := s.queryTable(q)
	if err != nil {
		return datasource.Content{}, err
	}

	total := len(rows)
	if !fullStmt {
		if n, err := s.count(fmt.Sprintf("SELECT COUNT(*) FROM %s%s", table, where)); err == nil {
			total = n
		}
	}

	pks, err := s.primaryKeys(container, entry)
	if err != nil {
		return datasource.Content{}, err
	}

	return datasource.Content{
		Kind:      datasource.KindTable,
		Type:      "table",
		Columns:   columns,
		Rows:      rows,
		CellTypes: cellTypes,
		Total:     total,
		ReadOnly:  pks,
		Query:     q,
	}, nil
}

// Update writes a single cell change via UPDATE matched on the table's primary
// key. Primary-key columns are not editable; tables without a primary key are
// read-only since a row can't be uniquely identified.
// Rename is not supported for mysql.
func (s *MySQLSource) Rename(_, _, _ string) error {
	return fmt.Errorf("rename not supported")
}

func (s *MySQLSource) Update(container, entry string, e datasource.Edit) error {
	if e.Column < 0 || e.Column >= len(e.Columns) {
		return fmt.Errorf("invalid column")
	}
	col := e.Columns[e.Column]

	pks, err := s.primaryKeys(container, entry)
	if err != nil {
		return err
	}
	if len(pks) == 0 {
		return fmt.Errorf("table has no primary key, not editable")
	}
	if slices.Contains(pks, col) {
		return fmt.Errorf("cannot edit primary key column %q", col)
	}

	// Locate each PK column in the row to build the WHERE predicate.
	colIdx := make(map[string]int, len(e.Columns))
	for i, c := range e.Columns {
		colIdx[c] = i
	}
	where := make([]string, 0, len(pks))
	args := make([]interface{}, 0, len(pks)+1)
	args = append(args, e.Value)
	for _, pk := range pks {
		i, ok := colIdx[pk]
		if !ok || i >= len(e.OldRow) {
			return fmt.Errorf("row missing primary key column %q", pk)
		}
		where = append(where, quoteIdent(pk)+" = ?")
		args = append(args, e.OldRow[i])
	}

	table := quoteIdent(container) + "." + quoteIdent(entry)
	q := fmt.Sprintf("UPDATE %s SET %s = ? WHERE %s", table, quoteIdent(col), strings.Join(where, " AND "))
	res, err := s.db.Exec(q, args...)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return fmt.Errorf("no row matched the primary key")
	}
	return nil
}

// DeleteRows removes rows matched on the table's primary key, one DELETE per
// row. Tables without a primary key are not deletable since a row can't be
// uniquely identified.
func (s *MySQLSource) DeleteRows(container, entry string, columns []string, rows []datasource.RowRef) error {
	pks, err := s.primaryKeys(container, entry)
	if err != nil {
		return err
	}
	if len(pks) == 0 {
		return fmt.Errorf("table has no primary key, not deletable")
	}
	colIdx := make(map[string]int, len(columns))
	for i, c := range columns {
		colIdx[c] = i
	}
	table := quoteIdent(container) + "." + quoteIdent(entry)
	for _, r := range rows {
		where := make([]string, 0, len(pks))
		args := make([]interface{}, 0, len(pks))
		for _, pk := range pks {
			i, ok := colIdx[pk]
			if !ok || i >= len(r.Row) {
				return fmt.Errorf("row missing primary key column %q", pk)
			}
			where = append(where, quoteIdent(pk)+" = ?")
			args = append(args, r.Row[i])
		}
		q := fmt.Sprintf("DELETE FROM %s WHERE %s", table, strings.Join(where, " AND "))
		res, err := s.db.Exec(q, args...)
		if err != nil {
			return err
		}
		if n, err := res.RowsAffected(); err == nil && n == 0 {
			return fmt.Errorf("no row matched the primary key")
		}
	}
	return nil
}

// DropEntry drops a table.
func (s *MySQLSource) DropEntry(container, entry string) error {
	_, err := s.db.Exec("DROP TABLE " + quoteIdent(container) + "." + quoteIdent(entry))
	return err
}

// DropContainer drops an entire database.
func (s *MySQLSource) DropContainer(container string) error {
	_, err := s.db.Exec("DROP DATABASE " + quoteIdent(container))
	return err
}

// Command runs a read-only SQL statement against the selected database and
// renders the result as aligned text. Writes are rejected so the command box
// can't mutate data; use inline editing for that.
func (s *MySQLSource) Command(container, raw string) (string, error) {
	q := strings.TrimSpace(raw)
	if q == "" {
		return "", fmt.Errorf("empty command")
	}
	if !isReadOnlyStmt(q) {
		return "", fmt.Errorf("only read-only statements are allowed (SELECT/SHOW/DESCRIBE/EXPLAIN)")
	}
	if container != "" {
		if _, err := s.db.Exec("USE " + quoteIdent(container)); err != nil {
			return "", err
		}
	}
	columns, rows, _, err := s.queryTable(q)
	if err != nil {
		return "", err
	}
	return renderTable(columns, rows), nil
}

// queryStrings runs a query whose first column is collected into a slice.
func (s *MySQLSource) queryStrings(q string) ([]string, error) {
	rows, err := s.db.Query(q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var v sql.NullString
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out = append(out, v.String)
	}
	return out, rows.Err()
}

// count runs a scalar COUNT(*) query.
func (s *MySQLSource) count(q string) (int, error) {
	var n int
	if err := s.db.QueryRow(q).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

// queryTable runs a SELECT-shaped query and returns its columns, string-rendered
// rows (NULL => ""), and per-cell database type names.
func (s *MySQLSource) queryTable(q string) (columns []string, rows [][]string, cellTypes [][]string, err error) {
	res, err := s.db.Query(q)
	if err != nil {
		return nil, nil, nil, err
	}
	defer res.Close()

	columns, err = res.Columns()
	if err != nil {
		return nil, nil, nil, err
	}
	colTypes, err := res.ColumnTypes()
	if err != nil {
		return nil, nil, nil, err
	}
	typeNames := make([]string, len(colTypes))
	for i, ct := range colTypes {
		typeNames[i] = strings.ToLower(ct.DatabaseTypeName())
	}

	for res.Next() {
		cells := make([]sql.RawBytes, len(columns))
		scan := make([]interface{}, len(columns))
		for i := range cells {
			scan[i] = &cells[i]
		}
		if err := res.Scan(scan...); err != nil {
			return nil, nil, nil, err
		}
		row := make([]string, len(columns))
		for i, c := range cells {
			if c == nil {
				row[i] = "" // NULL
			} else {
				row[i] = string(c)
			}
		}
		rows = append(rows, row)
		// type row is the same for every data row; reuse the shared slice.
		cellTypes = append(cellTypes, typeNames)
	}
	return columns, rows, cellTypes, res.Err()
}

// primaryKeys returns the primary-key column names of a table, in key order.
func (s *MySQLSource) primaryKeys(container, entry string) ([]string, error) {
	q := "SHOW KEYS FROM " + quoteIdent(container) + "." + quoteIdent(entry) + " WHERE Key_name = 'PRIMARY'"
	rows, err := s.db.Query(q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	seqIdx, nameIdx := -1, -1
	for i, c := range cols {
		switch c {
		case "Seq_in_index":
			seqIdx = i
		case "Column_name":
			nameIdx = i
		}
	}
	if nameIdx < 0 {
		return nil, fmt.Errorf("unexpected SHOW KEYS output")
	}

	type pk struct {
		seq  int
		name string
	}
	var pks []pk
	for rows.Next() {
		cells := make([]sql.RawBytes, len(cols))
		scan := make([]interface{}, len(cols))
		for i := range cells {
			scan[i] = &cells[i]
		}
		if err := rows.Scan(scan...); err != nil {
			return nil, err
		}
		seq := 0
		if seqIdx >= 0 {
			fmt.Sscanf(string(cells[seqIdx]), "%d", &seq)
		}
		pks = append(pks, pk{seq: seq, name: string(cells[nameIdx])})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Slice(pks, func(i, j int) bool { return pks[i].seq < pks[j].seq })
	names := make([]string, len(pks))
	for i, p := range pks {
		names[i] = p.name
	}
	return names, nil
}

// whereClause turns a user-typed query fragment into a SQL clause. Empty matches
// all. A fragment starting with WHERE (case-insensitive) is used verbatim; any
// other non-empty fragment is treated as a bare condition and prefixed with WHERE.
func whereClause(query string) string {
	q := strings.TrimSpace(query)
	if q == "" {
		return ""
	}
	if strings.HasPrefix(strings.ToUpper(q), "WHERE ") {
		return " " + q
	}
	return " WHERE " + q
}

// isReadOnlyStmt reports whether a statement is a safe read-only query.
func isReadOnlyStmt(q string) bool {
	head := strings.ToUpper(strings.Fields(q)[0])
	switch head {
	case "SELECT", "SHOW", "DESCRIBE", "DESC", "EXPLAIN":
		return true
	default:
		return false
	}
}

// quoteIdent backtick-quotes a SQL identifier, escaping embedded backticks.
func quoteIdent(name string) string {
	return "`" + strings.ReplaceAll(name, "`", "``") + "`"
}

// renderTable renders columns + rows as a simple aligned text grid for the
// command output pane.
func renderTable(columns []string, rows [][]string) string {
	if len(columns) == 0 {
		return "(empty)"
	}
	widths := make([]int, len(columns))
	for i, c := range columns {
		widths[i] = len(c)
	}
	for _, row := range rows {
		for i, c := range row {
			if i < len(widths) && len(c) > widths[i] {
				widths[i] = len(c)
			}
		}
	}
	var b strings.Builder
	writeRow := func(cells []string) {
		for i, c := range cells {
			if i > 0 {
				b.WriteString("  ")
			}
			b.WriteString(c)
			for pad := widths[i] - len(c); pad > 0; pad-- {
				b.WriteByte(' ')
			}
		}
		b.WriteByte('\n')
	}
	writeRow(columns)
	for _, row := range rows {
		writeRow(row)
	}
	return strings.TrimRight(b.String(), "\n")
}

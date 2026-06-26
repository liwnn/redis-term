package datasource

// Kind is the shape of an entry's content.
type Kind int

const (
	KindText  Kind = iota // a single text blob (e.g. a redis string)
	KindTable             // tabular rows (e.g. redis hash/list, mongo documents)
)

// Page bounds a content request for paginated sources.
type Page struct {
	Skip  int
	Limit int    // 0 means no limit
	Query string // backend-native filter (mongo: extended-JSON query document); empty => all
}

// Content is the right-pane payload for a selected entry, in a form both
// the terminal and web views can render without knowing the backend.
type Content struct {
	Kind      Kind     // Text or Table
	Type      string   // backend type label: "string"/"hash"/... or "collection"
	Columns   []string // table header (empty for Text)
	Rows      [][]string
	CellTypes [][]string // parallel to Rows; per-cell backend type. nil => treat all as string
	Text      string     // populated when Kind == KindText
	Total     int        // total rows/documents available (for paging UIs)
	ReadOnly  []string   // column names that cannot be edited (mysql PK, mongo _id). nil => all editable
	Query     string     // the full backend statement that produced this content
	// (mysql: SELECT ...; mongo: db.<coll>.find(...)), shown in the query box so
	// the user can edit and re-run it. Empty for backends without a query box.
}

// Edit describes a single table cell change for Datasource.Update.
type Edit struct {
	Columns []string // table header at edit time
	Row     int      // absolute row index (with paging offset applied)
	Column  int      // index of the changed column
	OldRow  []string // original cell values of the edited row, used to locate
	// the record (mongo _id / hash field / zset member / set member)
	OldRowTypes []string // per-cell types of OldRow (parallel to Columns); used to
	// coerce the mongo _id query key to its real type. nil => treat as string
	Value   string // new cell value (as typed)
	OldType string // original cell type from CellTypes; guides mongo coercion, ignored by redis
}

// RowRef identifies one row to delete by its original cell values and per-cell
// types (parallel to the columns passed to DeleteRows). Types guides mongo _id
// coercion; nil/empty => treat all cells as strings.
type RowRef struct {
	Row   []string
	Types []string
}

// Datasource is a tree-shaped backend: server > container > entry > content.
// Redis maps container=db, entry=key; Mongo maps container=database,
// entry=collection. Implementations are not required to be goroutine-safe;
// callers serialize access per connection.
type Datasource interface {
	// Open establishes the connection.
	Open() error
	// Containers lists the second level (redis dbs / mongo databases).
	Containers() ([]string, error)
	// Entries lists the third level within a container
	// (redis keys / mongo collections).
	Entries(container string) ([]string, error)
	// Content returns the right-pane payload for an entry.
	Content(container, entry string, page Page) (Content, error)
	// Update writes a single table cell change back to the backend.
	Update(container, entry string, e Edit) error
	// Rename changes an entry's key within a container (redis RENAME). Backends
	// without rename return an error.
	Rename(container, oldEntry, newEntry string) error
	// DeleteRows removes the given rows from an entry (mysql table rows / mongo
	// documents), located by primary key / _id. Backends without row-level
	// deletion return an error.
	DeleteRows(container, entry string, columns []string, rows []RowRef) error
	// DropEntry removes an entry (redis key / mongo collection) from a container.
	DropEntry(container, entry string) error
	// DropContainer removes a container (redis flushdb / mongo drop database).
	DropContainer(container string) error
	// Command runs a backend-native command (redis command / mongo query).
	Command(container, raw string) (string, error)
	// Close releases the connection.
	Close()
}

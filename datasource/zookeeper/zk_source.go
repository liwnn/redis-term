package zookeeper

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/go-zookeeper/zk"

	"github.com/liwnn/redisterm/datasource"
)

// maxWalkDepth bounds the recursive znode walk so a pathological tree can't
// produce an unbounded entry list.
const maxWalkDepth = 64

// ZKSource adapts a ZooKeeper connection to the Datasource interface. The znode
// tree is flattened to a path list: containers are the top-level znodes under
// "/", and entries are full descendant paths (e.g. "/app/config/db").
type ZKSource struct {
	addr string
	conn *zk.Conn
}

var _ datasource.Datasource = (*ZKSource)(nil)

// silentLogger discards the zk client's internal logging so connection churn
// doesn't bleed into the TUI/console.
type silentLogger struct{}

func (silentLogger) Printf(string, ...any) {}

// NewZKSource builds a source for a ZooKeeper host:port (default port 2181).
func NewZKSource(host string, port int) *ZKSource {
	if host == "" {
		host = "127.0.0.1"
	}
	if port == 0 {
		port = 2181
	}
	return &ZKSource{addr: fmt.Sprintf("%s:%d", host, port)}
}

func (s *ZKSource) Open() error {
	// zk.Connect returns before the session is established, so dial then poll
	// State() until a session exists. The short dial timeout keeps an
	// unreachable host from freezing the UI thread.
	conn, _, err := zk.Connect([]string{s.addr}, 5*time.Second, zk.WithLogger(silentLogger{}))
	if err != nil {
		return err
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if conn.State() == zk.StateHasSession {
			s.conn = conn
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	conn.Close()
	return fmt.Errorf("zookeeper: connect to %s timed out", s.addr)
}

func (s *ZKSource) Close() {
	if s.conn != nil {
		s.conn.Close()
	}
}

// Containers lists the top-level znodes directly under "/".
func (s *ZKSource) Containers() ([]string, error) {
	children, _, err := s.conn.Children("/")
	if err != nil {
		return nil, err
	}
	sort.Strings(children)
	return children, nil
}

// Entries returns the container root and all of its descendant paths, flattened
// into a sorted list (e.g. "/app", "/app/config", "/app/config/db").
func (s *ZKSource) Entries(container string) ([]string, error) {
	root := "/" + container
	var paths []string
	if err := s.walk(root, 0, &paths); err != nil {
		return nil, err
	}
	sort.Strings(paths)
	return paths, nil
}

// walk appends path and recurses into its children, depth-limited.
func (s *ZKSource) walk(path string, depth int, out *[]string) error {
	*out = append(*out, path)
	if depth >= maxWalkDepth {
		return nil
	}
	children, _, err := s.conn.Children(path)
	if err != nil {
		if err == zk.ErrNoNode {
			return nil // node vanished mid-walk; skip it
		}
		return err
	}
	for _, child := range children {
		next := path + "/" + child
		if path == "/" {
			next = "/" + child
		}
		if err := s.walk(next, depth+1, out); err != nil {
			return err
		}
	}
	return nil
}

// Content returns a znode's data as a single-cell "value" table plus its stat
// rendered into the query box. A one-column table (not text) is used so the
// value is editable in both the web and terminal UIs.
func (s *ZKSource) Content(_, entry string, _ datasource.Page) (datasource.Content, error) {
	data, _, err := s.conn.Get(entry)
	if err != nil {
		return datasource.Content{}, err
	}
	// A znode holds a single data blob, shown as editable multi-line text (no
	// table, no query box). Query is intentionally left empty.
	return datasource.Content{
		Kind:  datasource.KindText,
		Type:  "znode",
		Text:  string(data),
		Total: 1,
	}, nil
}

// statLine renders a znode's stat as a compact, read-only info string.
func statLine(entry string, stat *zk.Stat) string {
	if stat == nil {
		return entry
	}
	return fmt.Sprintf("%s  version=%d dataLen=%d children=%d ctime=%s mtime=%s",
		entry, stat.Version, stat.DataLength, stat.NumChildren,
		msTime(stat.Ctime), msTime(stat.Mtime))
}

// msTime formats a zk epoch-millisecond timestamp.
func msTime(ms int64) string {
	if ms == 0 {
		return "-"
	}
	return time.UnixMilli(ms).Format("2006-01-02 15:04:05")
}

// Update writes a znode's data blob from the edited text (e.Value holds the full
// new value). The write is unconditional (version -1) since this is an
// interactive browser and stale-version retries would just churn.
func (s *ZKSource) Update(_, entry string, e datasource.Edit) error {
	_, err := s.conn.Set(entry, []byte(e.Value), -1)
	return err
}

// Rename is not supported for zookeeper.
func (s *ZKSource) Rename(_, _, _ string) error {
	return fmt.Errorf("rename not supported")
}

// DeleteRows is not supported for zookeeper (znodes are edited as text, not rows).
func (s *ZKSource) DeleteRows(_, _ string, _ []string, _ []datasource.RowRef) error {
	return fmt.Errorf("row deletion not supported")
}

// DropEntry deletes a znode and its entire subtree. ZooKeeper refuses to delete
// a node that still has children, so children are removed depth-first before the
// node itself.
func (s *ZKSource) DropEntry(_, entry string) error {
	return s.deleteRecursive(entry, 0)
}

// DropContainer deletes a top-level znode and its entire subtree.
func (s *ZKSource) DropContainer(container string) error {
	return s.deleteRecursive("/"+container, 0)
}

// deleteRecursive removes path and all of its descendants, children first. The
// depth bound mirrors walk so a pathological tree can't recurse without limit.
func (s *ZKSource) deleteRecursive(path string, depth int) error {
	if depth < maxWalkDepth {
		children, _, err := s.conn.Children(path)
		if err != nil {
			if err == zk.ErrNoNode {
				return nil // already gone
			}
			return err
		}
		for _, child := range children {
			next := path + "/" + child
			if path == "/" {
				next = "/" + child
			}
			if err := s.deleteRecursive(next, depth+1); err != nil {
				return err
			}
		}
	}
	if err := s.conn.Delete(path, -1); err != nil && err != zk.ErrNoNode {
		return err
	}
	return nil
}

// Command runs a minimal read-only shell against the znode tree. Supported:
//
//	ls <path>     list a znode's children
//	get <path>    print a znode's data
//	stat <path>   print a znode's stat
//
// Writes are intentionally not exposed here; use inline editing for that.
func (s *ZKSource) Command(_, raw string) (string, error) {
	fields := strings.Fields(raw)
	if len(fields) == 0 {
		return "", fmt.Errorf("empty command")
	}
	cmd := strings.ToLower(fields[0])
	path := "/"
	if len(fields) > 1 {
		path = fields[1]
	}
	switch cmd {
	case "ls":
		children, _, err := s.conn.Children(path)
		if err != nil {
			return "", err
		}
		sort.Strings(children)
		return strings.Join(children, "\n"), nil
	case "get":
		data, _, err := s.conn.Get(path)
		if err != nil {
			return "", err
		}
		return string(data), nil
	case "stat":
		_, stat, err := s.conn.Get(path)
		if err != nil {
			return "", err
		}
		return statLine(path, stat), nil
	default:
		return "", fmt.Errorf("unsupported command %q (ls/get/stat only)", cmd)
	}
}

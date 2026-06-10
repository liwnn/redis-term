package config

import (
	"os"
	"path/filepath"
	"testing"
)

// writeTmp writes content to a temp config file and returns its path.
func writeTmp(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "redis-term.json")
	if err := os.WriteFile(p, []byte(content), 0666); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestLoadLegacyArray ensures the old bare-array format still loads.
func TestLoadLegacyArray(t *testing.T) {
	p := writeTmp(t, `[{"name":"a","host":"h","port":1},{"name":"b","host":"h2","port":2}]`)
	c, err := NewConfig(p)
	if err != nil {
		t.Fatal(err)
	}
	if c.Count() != 2 {
		t.Fatalf("Count = %d, want 2", c.Count())
	}
	if got := c.GetConfig(1).Name; got != "b" {
		t.Fatalf("GetConfig(1).Name = %q, want b", got)
	}
	// legacy files have no remembered selection
	if got := c.LastSelectedIndex(); got != 0 {
		t.Fatalf("LastSelectedIndex = %d, want 0", got)
	}
}

// TestLoadObjectFormat ensures the new object format with lastSelected loads.
func TestLoadObjectFormat(t *testing.T) {
	p := writeTmp(t, `{"connections":[{"name":"a"},{"name":"b"}],"lastSelected":"b"}`)
	c, err := NewConfig(p)
	if err != nil {
		t.Fatal(err)
	}
	if c.Count() != 2 {
		t.Fatalf("Count = %d, want 2", c.Count())
	}
	if got := c.LastSelectedIndex(); got != 1 {
		t.Fatalf("LastSelectedIndex = %d, want 1", got)
	}
}

// TestLastSelectedByName confirms the selection is resolved by name, surviving
// reordering, and falls back to 0 when the saved name is gone.
func TestLastSelectedByName(t *testing.T) {
	p := writeTmp(t, `{"connections":[{"name":"a"},{"name":"b"}],"lastSelected":"missing"}`)
	c, err := NewConfig(p)
	if err != nil {
		t.Fatal(err)
	}
	if got := c.LastSelectedIndex(); got != 0 {
		t.Fatalf("LastSelectedIndex with missing name = %d, want 0", got)
	}
}

// TestSaveRoundTrip verifies SaveLastSelected persists and reloads.
func TestSaveRoundTrip(t *testing.T) {
	p := writeTmp(t, `[{"name":"a"},{"name":"b"}]`)
	c, err := NewConfig(p)
	if err != nil {
		t.Fatal(err)
	}
	c.SaveLastSelected(1)

	c2, err := NewConfig(p)
	if err != nil {
		t.Fatal(err)
	}
	if got := c2.LastSelectedIndex(); got != 1 {
		t.Fatalf("after round-trip LastSelectedIndex = %d, want 1", got)
	}
	if c2.Count() != 2 {
		t.Fatalf("after round-trip Count = %d, want 2", c2.Count())
	}
}

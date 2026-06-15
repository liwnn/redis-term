package mongo

import "testing"

func TestShellFilter_LimitSuffix(t *testing.T) {
	cases := []struct {
		in    string
		want  string
		ok    bool
	}{
		{`db.users.find({})`, `{}`, true},
		{`db.users.find({}).limit(100)`, `{}`, true},
		{`db.users.find({"age":{"$gt":18}}).limit(50)`, `{"age":{"$gt":18}}`, true},
		{`db.users.find({"a":1}, {"b":1}).limit(10)`, `{"a":1}`, true},
		{`db.users.find()`, ``, true},
		{`{"a":1}`, ``, false},
	}
	for _, c := range cases {
		got, ok := shellFilter(c.in)
		if ok != c.ok || got != c.want {
			t.Errorf("shellFilter(%q) = (%q, %v), want (%q, %v)", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestShellLimit(t *testing.T) {
	cases := []struct {
		in   string
		want int64
		ok   bool
	}{
		{`db.users.find({}).limit(50)`, 50, true},
		{`db.users.find({"a":1}).limit(1000)`, 1000, true},
		{`db.users.find({})`, 0, false},
		{`db.users.find({}).limit(0)`, 0, false},
		{`db.users.find({}).limit(abc)`, 0, false},
		{`{"a":1}`, 0, false},
	}
	for _, c := range cases {
		got, ok := shellLimit(c.in)
		if ok != c.ok || got != c.want {
			t.Errorf("shellLimit(%q) = (%d, %v), want (%d, %v)", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestParseQuery_LimitSuffix(t *testing.T) {
	f, err := parseQuery(`db.users.find({"age":{"$gt":18}}).limit(50)`)
	if err != nil {
		t.Fatalf("parseQuery error: %v", err)
	}
	if f == nil {
		t.Fatal("expected non-nil filter")
	}
}

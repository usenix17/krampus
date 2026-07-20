package database

import "testing"

func TestBuildDSN(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"./database/krampus.db", "./database/krampus.db?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=on"},
		{"file:test.db?cache=shared", "file:test.db?cache=shared&_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=on"},
	}
	for _, c := range cases {
		if got := buildDSN(c.in); got != c.want {
			t.Errorf("buildDSN(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

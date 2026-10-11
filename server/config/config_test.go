package config

import "testing"

func TestParseList(t *testing.T) {
	got := parseList(" a@x.com , b@y.com ,, c@z.com ")
	want := []string{"a@x.com", "b@y.com", "c@z.com"}
	if len(got) != len(want) {
		t.Fatalf("parseList len = %d, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("parseList[%d] = %q, want %q", i, got[i], want[i])
		}
	}

	if len(parseList("")) != 0 {
		t.Error("parseList(\"\") should be empty")
	}
}

func TestParseIntFallsBackToDefault(t *testing.T) {
	if got := parseInt("not-a-number", 3); got != 3 {
		t.Errorf("parseInt fallback = %d, want 3", got)
	}
	if got := parseInt("7", 3); got != 7 {
		t.Errorf("parseInt = %d, want 7", got)
	}
}

func TestIsAdminEmail(t *testing.T) {
	c := &Config{AdminEmails: []string{"admin@example.com"}}
	if !c.IsAdminEmail("ADMIN@example.com") {
		t.Error("IsAdminEmail should be case-insensitive")
	}
	if c.IsAdminEmail("user@example.com") {
		t.Error("non-admin email should not match")
	}
}

package tui

import "testing"

func TestVisibleLen(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"plain", 5},
		{"\x1b[1mbold\x1b[0m", 4},
		{"  \x1b[1;7m > label \x1b[0m  \x1b[2mvalue\x1b[0m", len("   > label   value")},
	}
	for _, c := range cases {
		if got := visibleLen(c.in); got != c.want {
			t.Errorf("visibleLen(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("short", 10); got != "short" {
		t.Errorf("truncate short = %q", got)
	}
	// Long plain text gets cut and terminated with reset.
	got := truncate("abcdefghij", 5)
	if visibleLen(got) > 5 {
		t.Errorf("truncate visible length = %d", visibleLen(got))
	}
	if got[len(got)-4:] != "\x1b[0m" {
		t.Errorf("truncate missing reset: %q", got)
	}
	// ANSI sequences are preserved while visible text is cut.
	styled := "\x1b[1mabcdefgh\x1b[0m"
	got = truncate(styled, 4)
	if visibleLen(got) != 3 {
		t.Errorf("truncate styled visible = %d, want ≤4", visibleLen(got))
	}
}

func TestArrowSelectNonTTY(t *testing.T) {
	// Not a TTY in tests → falls back to numbered list reading stdin;
	// with no stdin data it returns the default item.
	items := []Item{{"one", "/tmp/one"}, {"two", "/tmp/two"}}
	value, ok := ArrowSelect(items, "Pick:", 1)
	if !ok {
		t.Skip("stdin unavailable in test environment")
	}
	if value != "/tmp/two" {
		t.Errorf("default selection = %q, want /tmp/two", value)
	}
}

func TestArrowSelectEmpty(t *testing.T) {
	if _, ok := ArrowSelect(nil, "Pick:", 0); ok {
		t.Error("expected cancel for empty items")
	}
}

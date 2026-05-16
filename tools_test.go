package main

import "testing"

func TestCalculator(t *testing.T) {
	cases := []struct {
		op   string
		a, b float64
		want string
	}{
		{"+", 1, 2, "3"},
		{"-", 5, 3, "2"},
		{"*", 4, 6, "24"},
		{"/", 10, 2, "5"},
		{"/", 1, 0, "error: divide by zero"},
		{"%", 1, 1, "error: unknown operator (use +, -, *, /)"},
	}
	for _, c := range cases {
		got := calculator(c.op, c.a, c.b)
		if got != c.want {
			t.Errorf("calculator(%q, %v, %v) = %q, want %q", c.op, c.a, c.b, got, c.want)
		}
	}
}

func TestSearch(t *testing.T) {
	got := search("gemini api")
	if got == "" {
		t.Fatal("expected non-empty result for known query")
	}
	miss := search("nothing-matches-this-query-xyz")
	if miss == "" {
		t.Fatal("expected non-empty fallback for unknown query")
	}
}

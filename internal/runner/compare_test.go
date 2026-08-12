package runner

import "testing"

func TestCompareOutput(t *testing.T) {
	for name, test := range map[string]struct {
		actual   string
		expected string
		match    bool
	}{
		"exact":               {"15\n", "15\n", true},
		"mismatch":            {"14\n", "15\n", false},
		"line endings":        {"one\r\ntwo\r\n", "one\ntwo\n", true},
		"trailing whitespace": {"one  \n two\t\n\n", "one\n two\n", true},
		"leading whitespace":  {" one\n", "one\n", false},
	} {
		t.Run(name, func(t *testing.T) {
			got := CompareOutput([]byte(test.actual), []byte(test.expected))
			if got.Match != test.match {
				t.Fatalf("CompareOutput(%q, %q).Match = %t, want %t", test.actual, test.expected, got.Match, test.match)
			}
		})
	}
}

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

func TestFirstOutputMismatch(t *testing.T) {
	for name, test := range map[string]struct {
		actual   string
		expected string
		want     *OutputMismatch
	}{
		"match":           {"one\n", "one\n", nil},
		"near start":      {"x\ntwo\n", "one\ntwo\n", &OutputMismatch{Line: 1, Expected: "one", Actual: "x"}},
		"middle":          {"one\nx\nthree\n", "one\ntwo\nthree\n", &OutputMismatch{Line: 2, Expected: "two", Actual: "x"}},
		"near end":        {"one\ntwo\nx\n", "one\ntwo\nthree\n", &OutputMismatch{Line: 3, Expected: "three", Actual: "x"}},
		"actual longer":   {"one\ntwo\n", "one\n", &OutputMismatch{Line: 2, ExpectedEnded: true, Actual: "two"}},
		"expected longer": {"one\n", "one\ntwo\n", &OutputMismatch{Line: 2, ActualEnded: true, Expected: "two"}},
		"empty actual":    {"", "one\n", &OutputMismatch{Line: 1, ActualEnded: true, Expected: "one"}},
		"normalization":   {"one \r\n\r\n", "one\n", nil},
	} {
		t.Run(name, func(t *testing.T) {
			got := FirstOutputMismatch([]byte(test.actual), []byte(test.expected))
			if !sameMismatch(got, test.want) {
				t.Fatalf("FirstOutputMismatch(%q, %q) = %#v, want %#v", test.actual, test.expected, got, test.want)
			}
		})
	}
}

func sameMismatch(got, want *OutputMismatch) bool {
	if got == nil || want == nil {
		return got == want
	}
	return *got == *want
}

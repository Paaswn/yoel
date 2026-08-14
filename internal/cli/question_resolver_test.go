package cli

import (
	"testing"

	"yoel/internal/graderapi"
)

func TestSearchProblemsPrioritizesExactMatchesAndSupportsMetadata(t *testing.T) {
	problems := []graderapi.Problem{
		{ID: 17, Name: "graph-basics", FullName: "Graph Basics", Tags: []string{"graph", "weighted"}},
		{ID: 1142, Name: "segment-tree", FullName: "Segment Tree", Tags: []string{"tree", "range queries"}},
		{ID: 1191, Name: "tree-traversal", FullName: "Tree Traversal", Tags: []string{"tree", "weighted"}},
	}
	for name, test := range map[string]struct {
		query string
		want  []int
	}{
		"exact ID wins over order": {"17", []int{17}},
		"order":                    {"2", []int{1142}},
		"case insensitive name":    {"SEGMENT-TREE", []int{1142}},
		"full name":                {"segment tree", []int{1142}},
		"prefix":                   {"seg", []int{1142}},
		"substring":                {"vers", []int{1191}},
		"tag":                      {"range", []int{1142}},
		"multiple tag matches":     {"weighted", []int{17, 1191}},
	} {
		t.Run(name, func(t *testing.T) {
			matches := searchProblems(problems, test.query)
			if len(matches) != len(test.want) {
				t.Fatalf("searchProblems(%q) = %#v, want IDs %#v", test.query, matches, test.want)
			}
			for index, problem := range matches {
				if problem.ID != test.want[index] {
					t.Fatalf("searchProblems(%q)[%d].ID = %d, want %d", test.query, index, problem.ID, test.want[index])
				}
			}
		})
	}
}

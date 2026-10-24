package core

import (
	"database/sql"
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestBestEvaluationLifecycle(t *testing.T) {
	r := newRegistryForTest(t)
	seedRegistryProblem(t, r, ProblemLite{ID: 42, CodeName: "sample", BestScore: 60})
	comment := func(s string) *string { return &s }
	for _, tc := range []struct {
		name       string
		score      float64
		evaluation *string
		wantScore  float64
		wantEval   *string
	}{
		{"lower cannot label historic best", 40, comment("[P---]"), 60, nil},
		{"tie fills missing evaluation", 60, comment("[PPP--]"), 60, comment("[PPP--]")},
		{"tie without evaluation retains it", 60, nil, 60, comment("[PPP--]")},
		{"lower cannot replace known evaluation", 50, comment("[PP---]"), 60, comment("[PPP--]")},
		{"higher without evaluation hides old one", 80, nil, 80, nil},
		{"tie fills new best", 80, comment("[PPPP-]"), 80, comment("[PPPP-]")},
		{"higher empty evaluation hides old one", 90, comment(""), 90, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := r.RecordProblemResult(42, tc.score, tc.evaluation, time.Time{}); err != nil {
				t.Fatal(err)
			}
			byID, err := r.QueryByID(42)
			if err != nil {
				t.Fatal(err)
			}
			byName, err := r.QueryByName("sample")
			if err != nil {
				t.Fatal(err)
			}
			all, err := r.GetAllProblems()
			if err != nil {
				t.Fatal(err)
			}
			for _, p := range append([]ProblemLite{byID}, append(byName, all...)...) {
				if p.BestScore != tc.wantScore || !reflect.DeepEqual(p.BestEvaluation, tc.wantEval) {
					t.Fatalf("score/evaluation = %v/%v, want %v/%v", p.BestScore, p.BestEvaluation, tc.wantScore, tc.wantEval)
				}
			}
		})
	}
}

func TestBestEvaluationSurvivesReopenAndSameScoreFetch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "registry.db")
	r, err := NewRegistryWithPath(path)
	if err != nil {
		t.Fatal(err)
	}
	p := ProblemLite{ID: 42, CodeName: "sample", BestScore: 0}
	if err := r.Upsert(p); err != nil {
		t.Fatal(err)
	}
	eval := "[----]"
	if err := r.RecordProblemResult(42, 0, &eval, time.Time{}); err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	r, err = NewRegistryWithPath(path)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	// fetch updates metadata but does not provide an evaluation.
	for _, score := range []float64{0, 100} {
		p.BestScore = score
		if err := r.Upsert(p); err != nil {
			t.Fatal(err)
		}
		got, err := r.QueryByID(42)
		if err != nil {
			t.Fatal(err)
		}
		if score == 0 && (got.BestEvaluation == nil || *got.BestEvaluation != eval) {
			t.Fatal("lost zero-score evaluation across reopen/fetch")
		}
		if score == 100 && got.BestEvaluation != nil {
			t.Fatal("fetch paired new score with old evaluation")
		}
	}
}

func TestEvaluationTableAddedToLegacyRegistry(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE problems (
		id INTEGER PRIMARY KEY, has_attachment INTEGER NOT NULL DEFAULT 0,
		sort_order INTEGER NOT NULL, best_score REAL, code_name TEXT NOT NULL,
		pretty_name TEXT NOT NULL, source_path TEXT, directory_path TEXT);
		INSERT INTO problems VALUES (42, 0, 1, 75, 'sample', 'Sample', '/sample/main.cpp', '/sample')`)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	r, err := NewRegistryWithPath(path)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	p, err := r.QueryByID(42)
	if err != nil {
		t.Fatal(err)
	}
	if p.BestScore != 75 || !p.IsOnLocal || p.SourcePath != "/sample/main.cpp" || p.BestEvaluation != nil {
		t.Fatalf("legacy data changed: %+v", p)
	}
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatal(err)
	}
	if _, exists := fields["best_evaluation"]; exists {
		t.Fatal("missing evaluation should be omitted from JSON")
	}
	eval := "[PPP-]"
	if err := r.RecordProblemResult(42, 75, &eval, time.Time{}); err != nil {
		t.Fatal(err)
	}
}

func TestEvaluationWriteFailureRollsBackScore(t *testing.T) {
	r := newRegistryForTest(t)
	seedRegistryProblem(t, r, ProblemLite{ID: 42, CodeName: "sample", BestScore: 50})
	if _, err := r.db.Exec("DROP TABLE problem_evaluations"); err != nil {
		t.Fatal(err)
	}
	eval := "[PPPP]"
	if err := r.RecordProblemResult(42, 100, &eval, time.Time{}); err == nil {
		t.Fatal("expected evaluation write failure")
	}
	var score float64
	if err := r.db.QueryRow("SELECT best_score FROM problems WHERE id = 42").Scan(&score); err != nil {
		t.Fatal(err)
	}
	if score != 50 {
		t.Fatalf("failed transaction changed best score to %v", score)
	}
}

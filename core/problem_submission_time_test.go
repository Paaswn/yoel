package core

import (
	"path/filepath"
	"testing"
	"time"
)

func TestBestSubmissionTimeTracksChosenResult(t *testing.T) {
	r := newRegistryForTest(t)
	seedRegistryProblem(t, r, ProblemLite{ID: 42, CodeName: "sample"})
	evaluation := "[PPP-]"
	blank := " \n "
	first := time.Date(2026, 4, 1, 12, 30, 0, 0, time.FixedZone("test", 7*60*60))
	for _, tc := range []struct {
		score     float64
		eval      *string
		submitted time.Time
		wantTime  time.Time
		wantEval  bool
	}{
		{60, &evaluation, first, first, true},
		{40, &evaluation, first.Add(time.Hour), first, true},
		{60, nil, first.Add(2 * time.Hour), first, true},
		{60, &blank, first.Add(2 * time.Hour), first, true},
		{60, &evaluation, first.Add(3 * time.Hour), first.Add(3 * time.Hour), true},
		{80, nil, first.Add(4 * time.Hour), first.Add(4 * time.Hour), false},
	} {
		if err := r.RecordProblemResult(42, tc.score, tc.eval, tc.submitted); err != nil {
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
			if p.BestSubmittedAt == nil || *p.BestSubmittedAt != tc.wantTime.UTC().Format(time.RFC3339Nano) {
				t.Fatalf("best submission time = %v, want %v", p.BestSubmittedAt, tc.wantTime)
			}
			if (p.BestEvaluation != nil) != tc.wantEval {
				t.Fatalf("unexpected best evaluation: %v", p.BestEvaluation)
			}
		}
	}
	if err := r.Upsert(ProblemLite{ID: 42, CodeName: "sample", BestScore: 100}); err != nil {
		t.Fatal(err)
	}
	p, err := r.QueryByID(42)
	if err != nil {
		t.Fatal(err)
	}
	if p.BestEvaluation != nil || p.BestSubmittedAt != nil {
		t.Fatal("fetch paired new best score with old submission details")
	}
}

func TestBestSubmissionTimeMigrationAndReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "registry.db")
	r, err := NewRegistryWithPath(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Upsert(ProblemLite{ID: 42, CodeName: "sample", BestScore: 75}); err != nil {
		t.Fatal(err)
	}
	// Recreate the previous evaluation-only schema to exercise the upgrade.
	if _, err := r.db.Exec(`DROP TABLE problem_evaluations;
		CREATE TABLE problem_evaluations (problem_id INTEGER PRIMARY KEY, score REAL NOT NULL, evaluation TEXT NOT NULL);
		INSERT INTO problem_evaluations VALUES (42, 75, '[PPP-]')`); err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	r, err = NewRegistryWithPath(path)
	if err != nil {
		t.Fatal(err)
	}
	p, err := r.QueryByID(42)
	if err != nil {
		t.Fatal(err)
	}
	if p.BestEvaluation == nil || *p.BestEvaluation != "[PPP-]" || p.BestSubmittedAt != nil {
		t.Fatalf("migration lost evaluation or invented time: %+v", p)
	}
	submitted := time.Date(2026, 4, 1, 10, 20, 30, 0, time.UTC)
	if err := r.RecordProblemResult(42, 75, p.BestEvaluation, submitted); err != nil {
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
	p, err = r.QueryByID(42)
	if err != nil {
		t.Fatal(err)
	}
	if p.BestSubmittedAt == nil || *p.BestSubmittedAt != submitted.Format(time.RFC3339Nano) {
		t.Fatalf("lost submission time after reopening: %+v", p)
	}
}

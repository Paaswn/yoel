/*
 * this test was mostly made by Codex
 */
package core

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	gapi "github.com/Paaswn/yoel/graderapi"
)

func newRegistryForTest(t *testing.T) *Registry {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "registry.db")
	r, err := NewRegistryWithPath(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
    	if err := r.Close(); err != nil {
            t.Errorf("closing registry: %v", err)
        }
	})
	return r
}

// Seed independently of Upsert so its failures do not obscure read regressions.
func seedRegistryProblem(t *testing.T, r *Registry, p ProblemLite) {
	t.Helper()
	var sourcePath, directoryPath any
	if p.IsOnLocal {
		sourcePath, directoryPath = p.SourcePath, p.DirectoryPath
	}
	_, err := r.db.Exec(`INSERT INTO problems
		(id, sort_order, best_score, code_name, pretty_name, source_path, directory_path)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.SortOrder, p.BestScore, p.CodeName, p.PrettyName, sourcePath, directoryPath)
	if err != nil {
		t.Fatal(err)
	}
}

func assertRegistryProblems(t *testing.T, got, want []ProblemLite) {
	t.Helper()
	// SortOrder is deliberately omitted from returned records.
	for i := range want {
		want[i].SortOrder = 0
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("problems = %#v, want %#v", got, want)
	}
}

func registryConfigDirForTest(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	// UserConfigDir uses XDG_CONFIG_HOME on Unix, HOME on macOS, and
	// AppData on Windows. All supported paths must stay inside root.
	t.Setenv("XDG_CONFIG_HOME", root)
	t.Setenv("HOME", root)
	t.Setenv("AppData", root)
	dir, err := os.UserConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	rel, err := filepath.Rel(root, dir)
	if err != nil || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		t.Fatalf("UserConfigDir = %q, outside temporary directory %q", dir, root)
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestRegistryNewRegistryPersistence(t *testing.T) {
	configDir := registryConfigDirForTest(t)
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.Close() })
	got, err := r.GetAllQuestions()
	if err != nil {
		t.Fatal(err)
	}
	assertRegistryProblems(t, got, []ProblemLite{})
	p := ProblemLite{ID: 42, SortOrder: 7, BestScore: 75.5, CodeName: "arrays", PrettyName: "Array Problem",
		SourcePath: "main.cpp", DirectoryPath: "arrays", IsOnLocal: true}
	seedRegistryProblem(t, r, p)
	if _, err := os.Stat(filepath.Join(configDir, yoelCache, yoelDatabase)); err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	got, err = reopened.GetAllQuestions()
	if err != nil {
		t.Fatal(err)
	}
	assertRegistryProblems(t, got, []ProblemLite{p})
}

func TestRegistryNewRegistryInvalidDirectory(t *testing.T) {
	dir := registryConfigDirForTest(t)
	if err := os.WriteFile(filepath.Join(dir, yoelCache), []byte("blocks directory creation"), 0600); err != nil {
		t.Fatal(err)
	}
	r, err := NewRegistry()
	if r != nil {
		_ = r.Close()
		t.Error("NewRegistry returned a registry on failure")
	}
	if err == nil {
		t.Fatal("NewRegistry succeeded with a file blocking its directory")
	}
}

func TestRegistryUpsert(t *testing.T) {
	t.Run("insert", func(t *testing.T) {
		r := newRegistryForTest(t)
		p := ProblemLite{ID: 42, SortOrder: 3, BestScore: 80.5, CodeName: "arrays", PrettyName: "Array Problem"}
		if err := r.Upsert(p); err != nil {
			t.Fatal(err)
		}
		got, err := r.GetAllQuestions()
		if err != nil {
			t.Fatal(err)
		}
		assertRegistryProblems(t, got, []ProblemLite{p})
		var order int
		if err := r.db.QueryRow("SELECT sort_order FROM problems WHERE id = ?", p.ID).Scan(&order); err != nil {
			t.Fatal(err)
		}
		if order != p.SortOrder {
			t.Errorf("stored order = %d, want %d", order, p.SortOrder)
		}
	})
	t.Run("update metadata and order while preserving local paths", func(t *testing.T) {
		r := newRegistryForTest(t)
		old := ProblemLite{ID: 42, SortOrder: 5, BestScore: 10, CodeName: "old", PrettyName: "Old Problem",
			SourcePath: "student.h", DirectoryPath: "local", IsOnLocal: true}
		seedRegistryProblem(t, r, old)
		other := ProblemLite{ID: 43, SortOrder: 2, CodeName: "other", PrettyName: "Other Problem"}
		seedRegistryProblem(t, r, other)
		updated := ProblemLite{ID: 42, SortOrder: 0, BestScore: 99.5, CodeName: "new", PrettyName: "New Problem"}
		if err := r.Upsert(updated); err != nil {
			t.Fatal(err)
		}
		updated.SourcePath, updated.DirectoryPath, updated.IsOnLocal = old.SourcePath, old.DirectoryPath, true
		got, err := r.GetAllQuestions()
		if err != nil {
			t.Fatal(err)
		}
		assertRegistryProblems(t, got, []ProblemLite{updated, other})
	})
}

func TestRegistryQueries(t *testing.T) {
	local := ProblemLite{ID: 42, SortOrder: 2, BestScore: 87.5, CodeName: "alpha-middle-code", PrettyName: "Middle Title",
		SourcePath: "main.cpp", DirectoryPath: "local", IsOnLocal: true}
	remote := ProblemLite{ID: 43, SortOrder: 0, CodeName: "remote", PrettyName: "A Middle Question"}
	unrelated := ProblemLite{ID: 44, SortOrder: 1, CodeName: "graphs", PrettyName: "Graph Problem"}
	r := newRegistryForTest(t)
	for _, p := range []ProblemLite{local, remote, unrelated} {
		seedRegistryProblem(t, r, p)
	}
	t.Run("ID fields and nullable paths", func(t *testing.T) {
		for _, want := range []ProblemLite{local, remote} {
			got, err := r.QueryByID(want.ID)
			if err != nil {
				t.Fatal(err)
			}
			assertRegistryProblems(t, []ProblemLite{got}, []ProblemLite{want})
		}
	})
	t.Run("missing ID", func(t *testing.T) {
		got, err := r.QueryByID(999)
		if !errors.Is(err, sql.ErrNoRows) || got != (ProblemLite{}) {
			t.Errorf("QueryByID = %#v, %v; want zero value, ProblemNotFound", got, err)
		}
	})
	t.Run("name substrings in either column without duplicates", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			want []ProblemLite
		}{
			{"middle", []ProblemLite{remote, local}},
			{"middle-code", []ProblemLite{local}},
			{"Middle Question", []ProblemLite{remote}},
		} {
			got, err := r.QueryByName(tc.name)
			if err != nil {
				t.Fatal(err)
			}
			assertRegistryProblems(t, got, tc.want)
		}
	})
	t.Run("unmatched name", func(t *testing.T) {
		got, err := r.QueryByName("missing")
		if !errors.Is(err, ErrProblemNotFound) || len(got) != 0 {
			t.Errorf("QueryByName = %#v, %v; want no records, ProblemNotFound", got, err)
		}
	})
	t.Run("all records ordered by stored order", func(t *testing.T) {
		got, err := r.GetAllQuestions()
		if err != nil {
			t.Fatal(err)
		}
		assertRegistryProblems(t, got, []ProblemLite{remote, unrelated, local})
	})
	t.Run("NULL score", func(t *testing.T) {
		if _, err := r.db.Exec("UPDATE problems SET best_score = NULL WHERE id = ?", remote.ID); err != nil {
			t.Fatal(err)
		}
		got, err := r.QueryByID(remote.ID)
		if err != nil {
			t.Fatal(err)
		}
		assertRegistryProblems(t, []ProblemLite{got}, []ProblemLite{remote})
		matches, err := r.QueryByName("remote")
		if err != nil {
			t.Fatal(err)
		}
		assertRegistryProblems(t, matches, []ProblemLite{remote})
		all, err := r.GetAllQuestions()
		if err != nil {
			t.Fatal(err)
		}
		assertRegistryProblems(t, all, []ProblemLite{remote, unrelated, local})
	})
}

func TestRegistryMutations(t *testing.T) {
	r := newRegistryForTest(t)
	p := ProblemLite{ID: 42, SortOrder: 8, BestScore: 12.5, CodeName: "arrays", PrettyName: "Array Problem"}
	seedRegistryProblem(t, r, p)
	if err := r.SetProblemPath(p.ID, "student.h", "arrays"); err != nil {
		t.Fatal(err)
	}
	p.SourcePath, p.DirectoryPath, p.IsOnLocal = "student.h", "arrays", true
	got, err := r.GetAllQuestions()
	if err != nil {
		t.Fatal(err)
	}
	assertRegistryProblems(t, got, []ProblemLite{p})
	if err := r.UpdateProblemScore(p.ID, 100); err != nil {
		t.Fatal(err)
	}
	p.BestScore = 100
	got, err = r.GetAllQuestions()
	if err != nil {
		t.Fatal(err)
	}
	assertRegistryProblems(t, got, []ProblemLite{p})
	// Missing-ID updates currently succeed without inserting a record.
	if err := r.SetProblemPath(999, "unused.cpp", "unused"); err != nil {
		t.Fatal(err)
	}
	if err := r.UpdateProblemScore(999, 50); err != nil {
		t.Fatal(err)
	}
	got, err = r.GetAllQuestions()
	if err != nil {
		t.Fatal(err)
	}
	assertRegistryProblems(t, got, []ProblemLite{p})
	var order int
	if err := r.db.QueryRow("SELECT sort_order FROM problems WHERE id = ?", p.ID).Scan(&order); err != nil {
		t.Fatal(err)
	}
	if order != p.SortOrder {
		t.Errorf("mutations changed stored order to %d, want %d", order, p.SortOrder)
	}
}

func TestRegistryClosedDatabaseErrors(t *testing.T) {
	r := newRegistryForTest(t)
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	for name, operation := range map[string]func() error{
		"init":   func() error { return r.init() },
		"upsert": func() error { return r.Upsert(ProblemLite{ID: 42}) },
		"path":   func() error { return r.SetProblemPath(42, "main.cpp", "local") },
		"score":  func() error { return r.UpdateProblemScore(42, 100) },
		"ID":     func() error { _, err := r.QueryByID(42); return err },
		"name":   func() error { _, err := r.QueryByName("arrays"); return err },
		"all":    func() error { _, err := r.GetAllQuestions(); return err },
	} {
		t.Run(name, func(t *testing.T) {
			if err := operation(); err == nil {
				t.Fatal("operation succeeded after Close")
			}
		})
	}
}

func TestRegistryUpdateAllQuestions(t *testing.T) {
	type refresher interface {
		updateAllQuestions(context.Context, *gapi.Client) error
	}
	if _, ok := any(&Registry{}).(refresher); !ok {
		t.Fatal("HTTP tests require updateAllQuestions(ctx context.Context, session SavedSession, client *gapi.Client) error; no request was sent")
	}
	const token = "fake-registry-token"
	newClient := func(t *testing.T, handler http.HandlerFunc) *gapi.Client {
		t.Helper()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if req.Method != http.MethodGet || req.URL.Path != "/api/v1/problems" {
				t.Errorf("request = %s %s, want GET /api/v1/problems", req.Method, req.URL.Path)
			}
			if req.Header.Get("Accept") != "application/json" || req.Header.Get("Authorization") != "Bearer "+token {
				t.Error("missing JSON acceptance or fake bearer authentication")
			}
			w.Header().Set("Content-Type", "application/json")
			handler(w, req)
		}))
		t.Cleanup(server.Close)
		client, err := gapi.NewClient(server.URL, server.Client())
		if err != nil {
			t.Fatal(err)
		}
		return client
	}
	refresh := func(r *Registry, client *gapi.Client) error {
		return any(r).(refresher).updateAllQuestions(t.Context(), client.WithToken(token))
	}
	t.Run("scores ordering repeated refresh and preserved paths", func(t *testing.T) {
		r := newRegistryForTest(t)
		local := ProblemLite{ID: 42, SortOrder: 4, BestScore: 10, CodeName: "old", PrettyName: "Old",
			SourcePath: "main.cpp", DirectoryPath: "arrays", IsOnLocal: true}
		seedRegistryProblem(t, r, local)
		body := `[{"id":43,"name":"graphs","full_name":"Graph Problem","best_score":null},{"id":42,"name":"arrays","full_name":"Array Problem","best_score":87.5}]`
		client := newClient(t, func(w http.ResponseWriter, _ *http.Request) { _, _ = fmt.Fprint(w, body) })
		if err := refresh(r, client); err != nil {
			t.Fatal(err)
		}
		local.CodeName, local.PrettyName, local.BestScore = "arrays", "Array Problem", 87.5
		remote := ProblemLite{ID: 43, CodeName: "graphs", PrettyName: "Graph Problem"}
		got, err := r.GetAllQuestions()
		if err != nil {
			t.Fatal(err)
		}
		assertRegistryProblems(t, got, []ProblemLite{remote, local})
		// Use a separate server for the next fixture to avoid shared mutable handler state.
		client = newClient(t, func(w http.ResponseWriter, _ *http.Request) {
			_, _ = fmt.Fprint(w, `[{"id":42,"name":"arrays","full_name":"Array Problem","best_score":100},{"id":43,"name":"graphs","full_name":"Graph Problem","best_score":null}]`)
		})
		if err := refresh(r, client); err != nil {
			t.Fatal(err)
		}
		local.BestScore = 100
		got, err = r.GetAllQuestions()
		if err != nil {
			t.Fatal(err)
		}
		assertRegistryProblems(t, got, []ProblemLite{local, remote})
	})
	t.Run("empty response", func(t *testing.T) {
		r := newRegistryForTest(t)
		client := newClient(t, func(w http.ResponseWriter, _ *http.Request) { _, _ = fmt.Fprint(w, "[]") })
		if err := refresh(r, client); err != nil {
			t.Fatal(err)
		}
		got, err := r.GetAllQuestions()
		if err != nil {
			t.Fatal(err)
		}
		assertRegistryProblems(t, got, []ProblemLite{})
	})
	for _, tc := range []struct {
		name   string
		status int
		body   string
		want   error
	}{
		{"unauthorized", http.StatusUnauthorized, "fake-private-body", gapi.ErrAuthentication},
		{"forbidden", http.StatusForbidden, "fake-private-body", gapi.ErrAuthentication},
		{"server failure", http.StatusInternalServerError, "fake-private-body", nil},
		{"malformed JSON", http.StatusOK, "not-json", gapi.ErrInvalidResponse},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newRegistryForTest(t)
			original := ProblemLite{ID: 42, BestScore: 75, CodeName: "arrays", PrettyName: "Array Problem"}
			seedRegistryProblem(t, r, original)
			client := newClient(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = fmt.Fprint(w, tc.body)
			})
			err := refresh(r, client)
			if err == nil {
				t.Fatal("refresh succeeded, want error")
			}
			if tc.want != nil && !errors.Is(err, tc.want) {
				t.Errorf("error = %v, want %v", err, tc.want)
			}
			if tc.status != http.StatusOK {
				var httpErr *gapi.HTTPError
				if !errors.As(err, &httpErr) || httpErr.StatusCode != tc.status {
					t.Errorf("error = %v, want HTTPError %d", err, tc.status)
				}
			}
			if strings.Contains(err.Error(), token) || strings.Contains(err.Error(), "fake-private-body") {
				t.Error("error exposed private request or response data")
			}
			got, err := r.GetAllQuestions()
			if err != nil {
				t.Fatal(err)
			}
			assertRegistryProblems(t, got, []ProblemLite{original})
		})
	}
	t.Run("database write failure", func(t *testing.T) {
		r := newRegistryForTest(t)
		if err := r.Close(); err != nil {
			t.Fatal(err)
		}
		client := newClient(t, func(w http.ResponseWriter, _ *http.Request) {
			_, _ = fmt.Fprint(w, `[{"id":42,"name":"arrays","full_name":"Array Problem","best_score":100}]`)
		})
		if err := refresh(r, client); err == nil {
			t.Fatal("refresh ignored a closed-database write failure")
		}
	})
}

package core

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	gapi "github.com/Paaswn/yoel/graderapi"
)

const questionTestPDF = "%PDF-1.7 test content"
const questionTestToken = "fake-question-token"

func TestCreateQuestionNoAttachment(t *testing.T) {
	cwd := t.TempDir()
	problem := ProblemLite{ID: 42, CodeName: "sample", HasAttachment: false}
	r := newRegistryForTest(t)
	if err := r.Upsert(problem); err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}
	client := newQuestionFilesTestClient(t, problem.ID, map[string]questionFileResponse{
		"pdf": {body: []byte(questionTestPDF)},
	}, "pdf")

	if err := createQuestion(t.Context(), cwd, problem, client, r); err != nil {
		t.Fatalf("createQuestion() error = %v", err)
	}

	questionDir := filepath.Join(cwd, problem.CodeName)
	assertFileContents(t, filepath.Join(questionDir, "main.cpp"), []byte(yoelSourceFile), 0o755)
	assertFileContents(t, filepath.Join(questionDir, "42.pdf"), []byte(questionTestPDF), 0o444)
	assertQuestionDirectory(t, filepath.Join(questionDir, yoelHiddenDir))
	assertQuestionRegistryPaths(t, r, problem.ID, filepath.Join(questionDir, "main.cpp"), questionDir)
	assertQuestionDirectoryEntries(t, cwd, problem.CodeName)
}

func TestCreateQuestionWithAttachment(t *testing.T) {
	cwd := t.TempDir()
	problem := ProblemLite{ID: 42, CodeName: "sample", HasAttachment: true}
	r := newRegistryForTest(t)
	if err := r.Upsert(problem); err != nil {
		t.Fatal(err)
	}
	entries := []questionZIPEntry{
		{name: "starter/"},
		{name: "starter/main.h", body: "// attached main header\n"},
		{name: "starter/student.h", body: "// attached student implementation\n"},
		{name: "starter/main.cpp", body: "// attached program, not the generated template\n"},
		{name: "README.txt", body: "attachment instructions\n"},
	}
	client := newQuestionFilesTestClient(t, problem.ID, map[string]questionFileResponse{
		"pdf":        {body: []byte(questionTestPDF)},
		"attachment": {body: makeQuestionTestZIP(t, entries)},
	}, "pdf", "attachment")

	if err := createQuestion(t.Context(), cwd, problem, client, r); err != nil {
		t.Fatalf("createQuestion() error = %v", err)
	}
	questionDir := filepath.Join(cwd, problem.CodeName)
	assertQuestionZIPContents(t, questionDir, entries)
	assertFileContents(t, filepath.Join(questionDir, "42.pdf"), []byte(questionTestPDF), 0o444)
	assertQuestionDirectory(t, filepath.Join(questionDir, yoelHiddenDir))
	assertQuestionRegistryPaths(t, r, problem.ID, filepath.Join(questionDir, "starter", "student.h"), questionDir)
	if _, err := os.Stat(filepath.Join(questionDir, "main.cpp")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("generated root main.cpp should be absent, stat error = %v", err)
	}
	got, err := r.QueryByID(problem.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !got.HasAttachment {
		t.Error("creation lost the attachment flag")
	}
	assertQuestionDirectoryEntries(t, cwd, problem.CodeName)
}

func TestCreateQuestionDoesNotReplaceExistingQuestion(t *testing.T) {
	cwd := t.TempDir()
	problem := ProblemLite{ID: 7, CodeName: "existing"}
	r := newRegistryForTest(t)
	if err := r.Upsert(problem); err != nil {
		t.Fatal(err)
	}
	previousDir := t.TempDir()
	previousSource := filepath.Join(previousDir, "student.h")
	if err := r.SetProblemPath(problem.ID, previousSource, previousDir); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(cwd, problem.CodeName)
	if err := os.Mkdir(destination, 0o755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(destination, "keep.txt")
	if err := os.WriteFile(marker, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	client := newQuestionFilesTestClient(t, problem.ID, map[string]questionFileResponse{
		"pdf": {body: []byte(questionTestPDF)},
	}, "pdf")
	if err := createQuestion(t.Context(), cwd, problem, client, r); err == nil {
		t.Error("creation succeeded, want error when destination exists")
	}
	assertFileContents(t, marker, []byte("keep"), 0o644)
	assertQuestionDirectoryEntries(t, destination, "keep.txt")
	assertQuestionDirectoryEntries(t, cwd, problem.CodeName)
	assertQuestionRegistryPaths(t, r, problem.ID, previousSource, previousDir)
}

func TestCreateQuestionDownloadFailures(t *testing.T) {
	corruptZIP := corruptQuestionTestZIP(t)
	for _, tc := range []struct {
		name       string
		pdf        questionFileResponse
		attachment questionFileResponse
		requests   []string
		wantErr    error
		wantStatus int
	}{
		{name: "PDF unauthorized", pdf: questionFileResponse{status: http.StatusUnauthorized}, requests: []string{"pdf"}, wantErr: gapi.ErrAuthentication, wantStatus: http.StatusUnauthorized},
		{name: "PDF server failure", pdf: questionFileResponse{status: http.StatusInternalServerError}, requests: []string{"pdf"}, wantStatus: http.StatusInternalServerError},
		{name: "invalid PDF", pdf: questionFileResponse{body: []byte("not a PDF")}, requests: []string{"pdf"}, wantErr: gapi.ErrInvalidResponse},
		{name: "attachment forbidden", attachment: questionFileResponse{status: http.StatusForbidden}, requests: []string{"pdf", "attachment"}, wantErr: gapi.ErrAuthentication, wantStatus: http.StatusForbidden},
		{name: "attachment server failure", attachment: questionFileResponse{status: http.StatusInternalServerError}, requests: []string{"pdf", "attachment"}, wantStatus: http.StatusInternalServerError},
		{name: "empty attachment body", requests: []string{"pdf", "attachment"}, wantErr: gapi.ErrInvalidResponse},
		{name: "invalid ZIP", attachment: questionFileResponse{body: []byte("not a ZIP")}, requests: []string{"pdf", "attachment"}, wantErr: zip.ErrFormat},
		{name: "corrupted ZIP entry", attachment: questionFileResponse{body: corruptZIP}, requests: []string{"pdf", "attachment"}, wantErr: zip.ErrChecksum},
		{name: "ZIP path traversal", attachment: questionFileResponse{body: makeQuestionTestZIP(t, []questionZIPEntry{{name: "../escape.cpp", body: "escape"}})}, requests: []string{"pdf", "attachment"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cwd := t.TempDir()
			problem := ProblemLite{ID: 42, CodeName: "sample", HasAttachment: true}
			r := newRegistryForTest(t)
			if err := r.Upsert(problem); err != nil {
				t.Fatal(err)
			}
			pdf := tc.pdf
			if len(tc.requests) == 2 {
				pdf.body = []byte(questionTestPDF)
			}
			client := newQuestionFilesTestClient(t, problem.ID, map[string]questionFileResponse{
				"pdf": pdf, "attachment": tc.attachment,
			}, tc.requests...)
			err := createQuestion(t.Context(), cwd, problem, client, r)
			if err == nil {
				t.Error("creation succeeded, want download or extraction error")
			} else {
				if tc.wantErr != nil && !errors.Is(err, tc.wantErr) {
					t.Errorf("error = %v, want %v", err, tc.wantErr)
				}
				if tc.wantStatus != 0 {
					var httpErr *gapi.HTTPError
					if !errors.As(err, &httpErr) || httpErr.StatusCode != tc.wantStatus {
						t.Errorf("error = %v, want HTTPError %d", err, tc.wantStatus)
					}
				}
				if strings.Contains(err.Error(), questionTestToken) {
					t.Error("error exposed the fake bearer token")
				}
			}
			assertQuestionDirectoryEntries(t, cwd)
			assertQuestionRegistryPaths(t, r, problem.ID, "", "")
		})
	}
}

func TestCreateQuestionCanceledContext(t *testing.T) {
	cwd := t.TempDir()
	problem := ProblemLite{ID: 42, CodeName: "sample", HasAttachment: true}
	r := newRegistryForTest(t)
	if err := r.Upsert(problem); err != nil {
		t.Fatal(err)
	}
	client := newQuestionFilesTestClient(t, problem.ID, nil)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := createQuestion(ctx, cwd, problem, client, r); !errors.Is(err, context.Canceled) {
		t.Errorf("error = %v, want context.Canceled", err)
	}
	assertQuestionDirectoryEntries(t, cwd)
	assertQuestionRegistryPaths(t, r, problem.ID, "", "")
}

func TestCreateQuestionMissingWorkingDirectory(t *testing.T) {
	root := t.TempDir()
	cwd := filepath.Join(root, "missing")
	problem := ProblemLite{ID: 42, CodeName: "sample"}
	r := newRegistryForTest(t)
	if err := r.Upsert(problem); err != nil {
		t.Fatal(err)
	}
	client := newQuestionFilesTestClient(t, problem.ID, map[string]questionFileResponse{
		"pdf": {body: []byte(questionTestPDF)},
	}, "pdf")
	if err := createQuestion(t.Context(), cwd, problem, client, r); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("error = %v, want missing-directory error", err)
	}
	assertQuestionDirectoryEntries(t, root)
	assertQuestionRegistryPaths(t, r, problem.ID, "", "")
}

func TestCreateQuestionRegistryUpdateFailure(t *testing.T) {
	cwd := t.TempDir()
	problem := ProblemLite{ID: 42, CodeName: "sample"}
	r := newRegistryForTest(t)
	if err := r.Upsert(problem); err != nil {
		t.Fatal(err)
	}
	// Reject the actual database write while leaving reads available so we can
	// verify that failed creation preserves the saved metadata.
	_, err := r.db.Exec(`CREATE TRIGGER reject_problem_paths
		BEFORE UPDATE OF source_path, directory_path ON problems
		BEGIN SELECT RAISE(FAIL, 'test rejected path update'); END`)
	if err != nil {
		t.Fatal(err)
	}
	client := newQuestionFilesTestClient(t, problem.ID, map[string]questionFileResponse{
		"pdf": {body: []byte(questionTestPDF)},
	}, "pdf")
	err = createQuestion(t.Context(), cwd, problem, client, r)
	if err == nil || !strings.Contains(err.Error(), "test rejected path update") {
		t.Errorf("error = %v, want rejected registry update", err)
	}
	assertQuestionDirectoryEntries(t, cwd, "sample")
	assertQuestionRegistryPaths(t, r, problem.ID, "", "")
}

func TestExtractQuestionIntoDir(t *testing.T) {
	for _, tc := range []struct {
		name    string
		entries []questionZIPEntry
		source  string
	}{
		{"student after main", []questionZIPEntry{{"nested/main.h", "main"}, {"nested/student.h", "student"}, {"README.txt", "readme"}}, "nested/student.h"},
		{"student before main", []questionZIPEntry{{"nested/student.h", "student"}, {"nested/main.h", "main"}}, "nested/student.h"},
		{"main before other files", []questionZIPEntry{{"nested/main.h", "main"}, {"main.cpp", "program"}, {"README.txt", "readme"}}, "nested/main.h"},
		{"main after other files", []questionZIPEntry{{"README.txt", "readme"}, {"nested/main.h", "main"}}, "nested/main.h"},
		{"last regular file fallback", []questionZIPEntry{{"main.cpp", "program"}, {"docs/README.txt", "readme"}, {"empty/", ""}}, "docs/README.txt"},
		{"empty ZIP", nil, ""},
		{"directory-only ZIP", []questionZIPEntry{{"empty/", ""}}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			source, err := extractQuestionIntoDir(dir, gapi.ProblemFile{Data: makeQuestionTestZIP(t, tc.entries)})
			if err != nil {
				t.Fatal(err)
			}
			if source != filepath.FromSlash(tc.source) {
				t.Errorf("source = %q, want %q", source, filepath.FromSlash(tc.source))
			}
			assertQuestionZIPContents(t, dir, tc.entries)
		})
	}
}

func TestExtractQuestionIntoDirFailures(t *testing.T) {
	for _, tc := range []struct {
		name    string
		data    []byte
		wantErr error
	}{
		{"invalid ZIP", []byte("invalid"), zip.ErrFormat},
		{"corrupt entry", corruptQuestionTestZIP(t), zip.ErrChecksum},
		{"parent traversal", makeQuestionTestZIP(t, []questionZIPEntry{{"../escape.cpp", "escape"}}), nil},
		{"nested traversal", makeQuestionTestZIP(t, []questionZIPEntry{{"nested/../../escape.cpp", "escape"}}), nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			dir := filepath.Join(root, "extract")
			if err := os.Mkdir(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			source, err := extractQuestionIntoDir(dir, gapi.ProblemFile{Data: tc.data})
			if err == nil {
				t.Error("extraction succeeded, want error")
			} else if tc.wantErr != nil && !errors.Is(err, tc.wantErr) {
				t.Errorf("error = %v, want %v", err, tc.wantErr)
			}
			if source != "" {
				t.Errorf("failed extraction returned source %q", source)
			}
			if _, err := os.Stat(filepath.Join(root, "escape.cpp")); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("extraction wrote outside destination, stat error = %v", err)
			}
		})
	}
	t.Run("blocked parent directory", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "nested"), []byte("keep"), 0o644); err != nil {
			t.Fatal(err)
		}
		data := makeQuestionTestZIP(t, []questionZIPEntry{{"nested/student.h", "source"}})
		if _, err := extractQuestionIntoDir(dir, gapi.ProblemFile{Data: data}); err == nil {
			t.Error("extraction ignored a file blocking the parent directory")
		}
		assertFileContents(t, filepath.Join(dir, "nested"), []byte("keep"), 0o644)
	})
	t.Run("file collides with directory", func(t *testing.T) {
		dir := t.TempDir()
		data := makeQuestionTestZIP(t, []questionZIPEntry{{"blocked/", ""}, {"blocked", "source"}})
		if _, err := extractQuestionIntoDir(dir, gapi.ProblemFile{Data: data}); err == nil {
			t.Error("extraction ignored a directory blocking the output file")
		}
		assertQuestionDirectory(t, filepath.Join(dir, "blocked"))
	})
}

func TestMakeEmptySourceFile(t *testing.T) {
	t.Run("template", func(t *testing.T) {
		dir := t.TempDir()
		if err := makeEmptySourceFile(dir); err != nil {
			t.Fatal(err)
		}
		assertFileContents(t, filepath.Join(dir, "main.cpp"), []byte(yoelSourceFile), 0o755)
	})
	t.Run("destination is a file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "blocked")
		if err := os.WriteFile(path, []byte("keep"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := makeEmptySourceFile(path); err == nil {
			t.Error("source creation ignored a file blocking its destination directory")
		}
		assertFileContents(t, path, []byte("keep"), 0o644)
	})
}

type questionZIPEntry struct {
	name string
	body string
}

func makeQuestionTestZIP(t *testing.T, entries []questionZIPEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)
	for _, entry := range entries {
		file, err := writer.CreateHeader(&zip.FileHeader{Name: entry.name, Method: zip.Store})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.Write([]byte(entry.body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func corruptQuestionTestZIP(t *testing.T) []byte {
	t.Helper()
	data := makeQuestionTestZIP(t, []questionZIPEntry{{"student.h", "source contents"}})
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	offset, err := reader.File[0].DataOffset()
	if err != nil {
		t.Fatal(err)
	}
	// Flip file data while retaining the ZIP directory and original checksum.
	data[offset] ^= 0xff
	return data
}

type questionFileResponse struct {
	status int
	body   []byte
}

func newQuestionFilesTestClient(t *testing.T, id int, responses map[string]questionFileResponse, wantRequests ...string) *gapi.Client {
	t.Helper()
	var mu sync.Mutex
	var requests []string
	prefix := fmt.Sprintf("/api/v1/problems/%d/files/", id)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		if !strings.HasPrefix(r.URL.Path, prefix) {
			t.Errorf("unexpected request path %q", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		kind := strings.TrimPrefix(r.URL.Path, prefix)
		mu.Lock()
		requests = append(requests, kind)
		mu.Unlock()
		response, ok := responses[kind]
		if !ok {
			t.Errorf("unexpected %s request", kind)
			http.NotFound(w, r)
			return
		}
		accept := "application/pdf"
		contentType := "application/pdf"
		if kind == "attachment" {
			accept, contentType = "application/zip, application/octet-stream", "application/zip"
		}
		if r.Header.Get("Accept") != accept {
			t.Errorf("%s Accept = %q, want %q", kind, r.Header.Get("Accept"), accept)
		}
		if r.Header.Get("Authorization") != "Bearer "+questionTestToken {
			t.Error("request missing fake bearer authentication")
		}
		w.Header().Set("Content-Type", contentType)
		status := response.status
		if status == 0 {
			status = http.StatusOK
		}
		w.WriteHeader(status)
		_, _ = w.Write(response.body)
	}))
	t.Cleanup(func() {
		server.Close()
		mu.Lock()
		defer mu.Unlock()
		if !reflect.DeepEqual(requests, wantRequests) {
			t.Errorf("requests = %v, want %v", requests, wantRequests)
		}
	})
	client, err := gapi.NewClient(server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	return client.WithToken(questionTestToken)
}

func assertQuestionRegistryPaths(t *testing.T, r *Registry, id int, source, dir string) {
	t.Helper()
	got, err := r.QueryByID(id)
	if err != nil {
		t.Fatal(err)
	}
	if got.SourcePath != source || got.DirectoryPath != dir || got.IsOnLocal != (dir != "") {
		t.Errorf("registry paths = source %q, directory %q, local %t; want %q, %q, %t",
			got.SourcePath, got.DirectoryPath, got.IsOnLocal, source, dir, dir != "")
	}
}

func assertQuestionDirectoryEntries(t *testing.T, dir string, want ...string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, entry := range entries {
		got = append(got, entry.Name())
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("directory %s entries = %v, want %v", dir, got, want)
	}
}

func assertQuestionDirectory(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() {
		t.Errorf("%s is not a directory", path)
	}
}

func assertQuestionZIPContents(t *testing.T, dir string, entries []questionZIPEntry) {
	t.Helper()
	for _, entry := range entries {
		path := filepath.Join(dir, filepath.FromSlash(entry.name))
		if strings.HasSuffix(entry.name, "/") {
			assertQuestionDirectory(t, path)
			continue
		}
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != entry.body {
			t.Errorf("contents of %s = %q, want %q", path, got, entry.body)
		}
	}
}

func assertFileContents(t *testing.T, path string, want []byte, wantMode os.FileMode) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if string(got) != string(want) {
		t.Errorf("contents of %s = %q, want %q", path, got, want)
	}
	assertFileMode(t, path, wantMode)
}

func assertFileMode(t *testing.T, path string, wantMode os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if gotMode := info.Mode().Perm(); gotMode != wantMode {
		t.Errorf("permissions of %s = %#o, want %#o", path, gotMode, wantMode)
	}
}

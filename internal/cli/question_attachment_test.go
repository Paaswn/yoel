package cli

import (
	"archive/zip"
	"bytes"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"yoel/internal/graderapi"

	"github.com/spf13/cobra"
)

func TestQuestionNewExtractsSingleAttachmentSourceAndCreatesReadPDF(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	workingDirectory := t.TempDir()
	t.Chdir(workingDirectory)

	pdf := []byte("%PDF-1.7\nattachment question")
	attachment := makeQuestionZIP(t, []questionZIPEntry{{Name: "template/starter.cpp", Data: "int main() { return 0; }\n"}})
	var attachmentRequests atomic.Int32
	server := newCLITestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer fake-token" {
			t.Errorf("Authorization = %q", got)
		}
		switch r.URL.Path {
		case "/api/v1/problems/673":
			_, _ = fmt.Fprintln(w, `{"id":673,"name":"starter","full_name":"Starter Problem","submission_count":0,"has_attachment":true,"submission_ids":[]}`)
		case "/api/v1/problems/673/files/pdf":
			w.Header().Set("Content-Type", "application/pdf")
			_, _ = w.Write(pdf)
		case "/api/v1/problems/673/files/attachment":
			attachmentRequests.Add(1)
			if got := r.Header.Get("Accept"); got != "application/zip, application/octet-stream" {
				t.Errorf("attachment Accept = %q", got)
			}
			w.Header().Set("Content-Type", "application/zip")
			_, _ = w.Write(attachment)
		default:
			t.Errorf("unexpected request = %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	if err := saveSession(storedSession{
		BaseURL:   server.URL,
		Token:     "fake-token",
		ExpiresAt: time.Now().Add(time.Hour),
		User:      graderapi.User{ID: 7, Login: "fake-login", FullName: "Fake Student"},
	}); err != nil {
		t.Fatal(err)
	}

	var openedPath string
	command := newRootCommandWithOpener(func(_ *cobra.Command) (string, string, error) {
		t.Fatal("question new unexpectedly prompted for credentials")
		return "", "", nil
	}, func(path string) error {
		openedPath = path
		return nil
	})
	command.SetOut(new(bytes.Buffer))
	command.SetErr(new(bytes.Buffer))
	command.SetArgs([]string{"question", "new", "673"})
	if err := command.Execute(); err != nil {
		t.Fatalf("question new: %v", err)
	}

	problemDir := filepath.Join(workingDirectory, "starter")
	source, err := os.ReadFile(filepath.Join(problemDir, "673.cpp"))
	if err != nil {
		t.Fatalf("read renamed attachment source: %v", err)
	}
	if got, want := string(source), "int main() { return 0; }\n"; got != want {
		t.Fatalf("source = %q, want %q", got, want)
	}
	readPDF, err := os.ReadFile(filepath.Join(problemDir, "Read.pdf"))
	if err != nil {
		t.Fatalf("read Read.pdf: %v", err)
	}
	if !bytes.Equal(readPDF, pdf) {
		t.Fatalf("Read.pdf = %q, want cached statement", readPDF)
	}
	if openedPath == "" || filepath.Ext(openedPath) != ".pdf" {
		t.Fatalf("opened path = %q, want cached PDF", openedPath)
	}
	if attachmentRequests.Load() != 1 {
		t.Fatalf("attachment requests = %d, want 1", attachmentRequests.Load())
	}
	if _, err := os.Stat(filepath.Join(workingDirectory, "673.cpp")); !os.IsNotExist(err) {
		t.Fatalf("unexpected source in current directory: %v", err)
	}
	if _, err := os.Stat(filepath.Join(problemDir, "template")); !os.IsNotExist(err) {
		t.Fatalf("empty archive directory was not removed: %v", err)
	}
}

func TestQuestionListExtractsSelectedAttachment(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("TERM", "dumb")
	workingDirectory := t.TempDir()
	t.Chdir(workingDirectory)

	attachment := makeQuestionZIP(t, []questionZIPEntry{{Name: "starter.cpp", Data: "int main() {}\n"}})
	server := newCLITestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/problems":
			_, _ = fmt.Fprintln(w, `[{"id":42,"name":"arrays","full_name":"Array Problem","has_attachment":true}]`)
		case "/api/v1/problems/42/files/pdf":
			_, _ = fmt.Fprintln(w, "%PDF-1.7")
		case "/api/v1/problems/42/files/attachment":
			_, _ = w.Write(attachment)
		default:
			t.Errorf("unexpected request = %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	if err := saveSession(storedSession{
		BaseURL:   server.URL,
		Token:     "fake-token",
		ExpiresAt: time.Now().Add(time.Hour),
		User:      graderapi.User{ID: 7, Login: "fake-login", FullName: "Fake Student"},
	}); err != nil {
		t.Fatal(err)
	}
	command := newRootCommandWithOpener(func(_ *cobra.Command) (string, string, error) {
		t.Fatal("question list unexpectedly prompted for credentials")
		return "", "", nil
	}, func(string) error { return nil })
	command.SetIn(strings.NewReader("1\n"))
	command.SetOut(new(bytes.Buffer))
	command.SetErr(new(bytes.Buffer))
	command.SetArgs([]string{"question", "list"})
	if err := command.Execute(); err != nil {
		t.Fatalf("question list: %v", err)
	}

	if _, err := os.Stat(filepath.Join(workingDirectory, "arrays", "42.cpp")); err != nil {
		t.Fatalf("stat extracted list source: %v", err)
	}
}

func TestExtractQuestionAttachmentPreservesMultipleFiles(t *testing.T) {
	archive := makeQuestionZIP(t, []questionZIPEntry{
		{Name: "src/main.cpp", Data: "int main() {}\n"},
		{Name: "include/helper.h", Data: "#pragma once\n"},
	})
	destination := t.TempDir()
	files, err := extractQuestionAttachment(archive, destination)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("files = %#v, want 2", files)
	}
	for name, want := range map[string]string{
		filepath.Join("src", "main.cpp"):     "int main() {}\n",
		filepath.Join("include", "helper.h"): "#pragma once\n",
	} {
		data, err := os.ReadFile(filepath.Join(destination, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if string(data) != want {
			t.Fatalf("%s = %q, want %q", name, data, want)
		}
	}
}

func TestExtractQuestionAttachmentRejectsUnsafePathsAndSymlinks(t *testing.T) {
	for _, name := range []string{"../escape.cpp", "/absolute.cpp", `..\escape.cpp`, "C:/escape.cpp", "src/file:stream", "NUL.cpp", "LONGDI~1/file.cpp"} {
		t.Run(strings.ReplaceAll(name, "/", "_"), func(t *testing.T) {
			archive := makeQuestionZIP(t, []questionZIPEntry{{Name: name, Data: "private source"}})
			if _, err := extractQuestionAttachment(archive, t.TempDir()); err == nil || !strings.Contains(err.Error(), "unsafe path") {
				t.Fatalf("error = %v, want unsafe path", err)
			}
		})
	}

	readPDFArchive := makeQuestionZIP(t, []questionZIPEntry{{Name: "Read.pdf", Data: "single source"}})
	if files, err := extractQuestionAttachment(readPDFArchive, t.TempDir()); err != nil || len(files) != 1 {
		t.Fatalf("single Read.pdf source: files=%#v error=%v", files, err)
	}

	symlinkArchive := makeQuestionZIP(t, []questionZIPEntry{{Name: "link.cpp", Data: "../outside", Mode: os.ModeSymlink | 0o777}})
	if _, err := extractQuestionAttachment(symlinkArchive, t.TempDir()); err == nil || !strings.Contains(err.Error(), "unsupported entry") {
		t.Fatalf("symlink error = %v, want unsupported entry", err)
	}

	collisionArchive := makeQuestionZIP(t, []questionZIPEntry{{Name: "Source.cpp", Data: "one"}, {Name: "source.cpp", Data: "two"}})
	if _, err := extractQuestionAttachment(collisionArchive, t.TempDir()); err == nil || !strings.Contains(err.Error(), "duplicate path") {
		t.Fatalf("collision error = %v, want duplicate path", err)
	}

	destination := t.TempDir()
	lateUnsafeArchive := makeQuestionZIP(t, []questionZIPEntry{{Name: "safe.cpp", Data: "safe"}, {Name: "../escape.cpp", Data: "unsafe"}})
	if _, err := extractQuestionAttachment(lateUnsafeArchive, destination); err == nil {
		t.Fatal("late unsafe path was accepted")
	}
	if _, err := os.Stat(filepath.Join(destination, "safe.cpp")); !os.IsNotExist(err) {
		t.Fatalf("preflight wrote a file before rejecting archive: %v", err)
	}
}

func TestRenameSingleAttachmentSourceMovesSourceOutOfBlockingDirectory(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "673.cpp")
	source := filepath.Join(target, "nested", "source.cpp")
	if err := os.MkdirAll(filepath.Dir(source), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("source"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := renameSingleAttachmentSource(source, target, root); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "source" {
		t.Fatalf("renamed source = %q", data)
	}
}

func TestExtractQuestionAttachmentRejectsExcessiveEntries(t *testing.T) {
	entries := make([]questionZIPEntry, maxAttachmentEntries+1)
	for i := range entries {
		entries[i] = questionZIPEntry{Name: fmt.Sprintf("dir-%d/", i), Mode: os.ModeDir | 0o755}
	}
	archive := makeQuestionZIP(t, entries)
	if _, err := extractQuestionAttachment(archive, t.TempDir()); err == nil || !strings.Contains(err.Error(), "entries") {
		t.Fatalf("error = %v, want entry limit", err)
	}
}

type questionZIPEntry struct {
	Name string
	Data string
	Mode os.FileMode
}

func makeQuestionZIP(t *testing.T, entries []questionZIPEntry) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for _, entry := range entries {
		header := &zip.FileHeader{Name: entry.Name, Method: zip.Deflate}
		if entry.Mode != 0 {
			header.SetMode(entry.Mode)
		}
		file, err := writer.CreateHeader(header)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.Write([]byte(entry.Data)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

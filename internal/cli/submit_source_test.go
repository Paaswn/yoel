package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"yoel/internal/graderapi"
	"yoel/internal/registry"

	"github.com/spf13/cobra"
)

func TestSubmitCommandResolvesSourceInsideDirectory(t *testing.T) {
	workspace := t.TempDir()
	sourcePath := filepath.Join(workspace, "nested", "673.cpp")
	if err := os.MkdirAll(filepath.Dir(sourcePath), 0o755); err != nil {
		t.Fatal(err)
	}
	const source = "int main() { return 0; }\n"
	if err := os.WriteFile(sourcePath, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}

	server := newCLITestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/problems/673/submissions":
			var request graderapi.SubmissionRequest
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Errorf("decode request: %v", err)
			}
			if request.Source != source || request.Filename != "673.cpp" {
				t.Errorf("request = %#v", request)
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = fmt.Fprintln(w, `{"id":924618,"number":1,"status":"submitted"}`)
		case "/api/v1/submissions/924618":
			_, _ = fmt.Fprintln(w, `{
				"id":924618,"problem_id":673,"language":"cpp",
				"submitted_at":"2030-01-02T03:04:05Z","points":100,
				"status":"done","number":1
			}`)
		default:
			t.Errorf("unexpected request = %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	command := newSubmitCommand(func(*cobra.Command) (storedSession, error) {
		return storedSession{
			BaseURL:   server.URL,
			Token:     "fake-token",
			ExpiresAt: time.Now().Add(time.Hour),
			User:      graderapi.User{ID: 7},
		}, nil
	})
	command.SetOut(new(bytes.Buffer))
	command.SetErr(new(bytes.Buffer))
	command.SetArgs([]string{workspace})
	if err := command.Execute(); err != nil {
		t.Fatalf("submit directory: %v", err)
	}
}

func TestResolveSubmissionSourceUsesExistingFile(t *testing.T) {
	root := t.TempDir()
	sourcePath := filepath.Join(root, "1142.py")
	if err := os.WriteFile(sourcePath, []byte("print('ok')\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	path, filename, problemID, err := resolveSubmissionSource(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	if path != sourcePath || filename != "1142.py" || problemID != 1142 {
		t.Fatalf("resolved = (%q, %q, %d)", path, filename, problemID)
	}
}

func TestResolveSubmissionSourceFindsIDCPPInsideDirectory(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "FullName_Dir")
	sourcePath := filepath.Join(workspace, "nested", "673.cpp")
	if err := os.MkdirAll(filepath.Dir(sourcePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sourcePath, []byte("int main() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "Read.pdf"), []byte("PDF"), 0o600); err != nil {
		t.Fatal(err)
	}

	path, filename, problemID, err := resolveSubmissionSource(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if path != sourcePath || filename != "673.cpp" || problemID != 673 {
		t.Fatalf("resolved = (%q, %q, %d), want nested source", path, filename, problemID)
	}
}

func TestResolveSubmissionSourceSearchesCurrentDirectoryForMissingFilename(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	sourcePath := filepath.Join(root, "problem", "src", "673.cpp")
	if err := os.MkdirAll(filepath.Dir(sourcePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sourcePath, []byte("int main() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	path, filename, problemID, err := resolveSubmissionSource("673.cpp")
	if err != nil {
		t.Fatal(err)
	}
	if path != sourcePath || filename != "673.cpp" || problemID != 673 {
		t.Fatalf("resolved = (%q, %q, %d), want recursively found source", path, filename, problemID)
	}
}

func TestResolveSubmissionSourceDirectoryRequiresLowercaseIDCPP(t *testing.T) {
	workspace := t.TempDir()
	for _, name := range []string{"673.CPP", "674.py", "name.cpp", "0.cpp"} {
		if err := os.WriteFile(filepath.Join(workspace, name), []byte("source"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, _, err := resolveSubmissionSource(workspace); !errors.Is(err, errSubmissionSourceNotFound) {
		t.Fatalf("error = %v, want errSubmissionSourceNotFound", err)
	}
}

func TestResolveSubmissionSourceDoesNotFollowSymlinkDirectory(t *testing.T) {
	root := t.TempDir()
	external := t.TempDir()
	if err := os.WriteFile(filepath.Join(external, "673.cpp"), []byte("source"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, filepath.Join(root, "linked")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, _, _, err := resolveSubmissionSource(root); !errors.Is(err, errSubmissionSourceNotFound) {
		t.Fatalf("error = %v, want errSubmissionSourceNotFound", err)
	}
}

func TestResolveSubmissionSourceRejectsAmbiguousDirectory(t *testing.T) {
	workspace := t.TempDir()
	for _, name := range []string{"673.cpp", filepath.Join("nested", "674.cpp")} {
		path := filepath.Join(workspace, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("source"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	if _, _, _, err := resolveSubmissionSource(workspace); !errors.Is(err, errAmbiguousSubmissionFile) {
		t.Fatalf("error = %v, want errAmbiguousSubmissionFile", err)
	}
}

func TestResolveSubmissionSourceRejectsAmbiguousRecursiveFilename(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	for _, directory := range []string{"first", "second"} {
		path := filepath.Join(root, directory, "673.cpp")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("source"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	if _, _, _, err := resolveSubmissionSource("673.cpp"); !errors.Is(err, errAmbiguousSubmissionFile) {
		t.Fatalf("error = %v, want errAmbiguousSubmissionFile", err)
	}
}

func TestResolveSubmissionSourceReportsMissingFile(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	for _, name := range []string{"0673.cpp", "673.CPP"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("distractor"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, _, err := resolveSubmissionSource("673.cpp"); !errors.Is(err, errSubmissionSourceNotFound) {
		t.Fatalf("error = %v, want errSubmissionSourceNotFound", err)
	}
}

func TestResolveSubmissionSourceWithRegistryUsesRegisteredKeys(t *testing.T) {
	root := t.TempDir()
	directory := filepath.Join(root, "cpp_basics_1")
	sourcePath := filepath.Join(directory, "main.cpp")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sourcePath, []byte("int main() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	questions := registry.Registry{
		567: {
			ID:            567,
			Name:          "cpp_basics_1",
			FullName:      "C++ Basics 1",
			DirectoryPath: directory,
			SourcePath:    sourcePath,
		},
	}
	for _, key := range []string{"main.cpp", "cpp_basics_1", directory, "567", "C++ Basics 1"} {
		resolved, err := resolveSubmissionSourceWithRegistry(key, questions)
		if err != nil {
			t.Fatalf("resolve %q: %v", key, err)
		}
		if resolved.Path != sourcePath || resolved.Filename != "main.cpp" || resolved.ProblemID != 567 || resolved.UsedLegacy {
			t.Fatalf("resolve %q = %#v", key, resolved)
		}
	}
}

func TestResolveSubmissionSourceWithRegistryPreservesExplicitPathAndLegacyFallback(t *testing.T) {
	root := t.TempDir()
	directPath := filepath.Join(root, "567.cpp")
	if err := os.WriteFile(directPath, []byte("int main() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	direct, err := resolveSubmissionSourceWithRegistry(directPath, registry.Registry{})
	if err != nil {
		t.Fatal(err)
	}
	if direct.Path != directPath || direct.ProblemID != 567 || direct.UsedLegacy {
		t.Fatalf("direct = %#v", direct)
	}

	searchRoot := t.TempDir()
	t.Chdir(searchRoot)
	legacyPath := filepath.Join(searchRoot, "nested", "673.cpp")
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacyPath, []byte("int main() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	legacy, err := resolveSubmissionSourceWithRegistry("673.cpp", registry.Registry{})
	if err != nil {
		t.Fatal(err)
	}
	if legacy.Path != legacyPath || !legacy.UsedLegacy {
		t.Fatalf("legacy = %#v", legacy)
	}
}

func TestResolveSubmissionSourceWithRegistryDoesNotFallbackForKnownMissingOrStaleEntry(t *testing.T) {
	questions := registry.Registry{
		567: {ID: 567, Name: "remote-only"},
		891: {ID: 891, Name: "stale", SourcePath: filepath.Join(t.TempDir(), "missing.cpp")},
	}
	if _, err := resolveSubmissionSourceWithRegistry("567", questions); !errors.Is(err, registry.ErrNoLocalSource) {
		t.Fatalf("missing source error = %v", err)
	}
	if _, err := resolveSubmissionSourceWithRegistry("891", questions); !errors.Is(err, registry.ErrStaleSource) {
		t.Fatalf("stale source error = %v", err)
	}
}

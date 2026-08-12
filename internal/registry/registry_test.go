package registry

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMissingRegistryReturnsEmpty(t *testing.T) {
	entries, err := Load(filepath.Join(t.TempDir(), "missing", "questions.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("entries = %#v, want empty", entries)
	}
}

func TestLoadRejectsMalformedRegistry(t *testing.T) {
	path := filepath.Join(t.TempDir(), "questions.json")
	if err := os.WriteFile(path, []byte("not-json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("malformed registry was accepted")
	}
}

func TestSaveReloadAndMergeRemoteMetadata(t *testing.T) {
	directory := t.TempDir()
	source := createSource(t, directory, "567.cpp")
	registry := Registry{}
	if err := registry.UpsertRemote(567, "old-name", "Old Name"); err != nil {
		t.Fatal(err)
	}
	if err := registry.BindLocal(567, directory, source); err != nil {
		t.Fatal(err)
	}
	if err := registry.UpsertRemote(567, "new-name", "New Name"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "nested", "questions.json")
	if err := registry.Save(path); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	entry := loaded[567]
	if entry.Name != "new-name" || entry.FullName != "New Name" || entry.DirectoryPath != directory || entry.SourcePath != source {
		t.Fatalf("entry = %#v", entry)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("registry mode = %o, want 600", info.Mode().Perm())
	}
}

func TestBindLocalNormalizesRelativePaths(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	if err := os.Mkdir("question", 0o700); err != nil {
		t.Fatal(err)
	}
	source := createSource(t, filepath.Join(root, "question"), "567.cpp")
	registry := Registry{}
	if err := registry.UpsertRemote(567, "arrays", "Arrays"); err != nil {
		t.Fatal(err)
	}
	if err := registry.BindLocal(567, "question", filepath.Join("question", "567.cpp")); err != nil {
		t.Fatal(err)
	}
	entry := registry[567]
	if entry.DirectoryPath != filepath.Join(root, "question") || entry.SourcePath != source {
		t.Fatalf("entry = %#v", entry)
	}
}

func TestResolveExactRegistryKeys(t *testing.T) {
	root := t.TempDir()
	firstDir := filepath.Join(root, "cpp_basics_1")
	secondDir := filepath.Join(root, "vectors")
	firstSource := createSource(t, firstDir, "567.cpp")
	secondSource := createSource(t, secondDir, "891.cpp")
	registry := Registry{
		567: {ID: 567, Name: "cpp_basics_1", FullName: "C++ Basics 1", DirectoryPath: firstDir, SourcePath: firstSource},
		891: {ID: 891, Name: "vectors", FullName: "Vector Intro", DirectoryPath: secondDir, SourcePath: secondSource},
	}
	for _, key := range []string{"567.cpp", "cpp_basics_1", firstDir, "567", "C++ Basics 1"} {
		result, err := registry.Resolve(key)
		if err != nil {
			t.Fatalf("Resolve(%q): %v", key, err)
		}
		if result.Entry.ID != 567 || result.SourcePath != firstSource {
			t.Fatalf("Resolve(%q) = %#v", key, result)
		}
	}
}

func TestResolveRejectsAmbiguousFilenameAndName(t *testing.T) {
	root := t.TempDir()
	firstDir := filepath.Join(root, "first")
	secondDir := filepath.Join(root, "second")
	registry := Registry{
		567: {ID: 567, Name: "same", DirectoryPath: firstDir, SourcePath: createSource(t, firstDir, "main.cpp")},
		891: {ID: 891, Name: "same", DirectoryPath: secondDir, SourcePath: createSource(t, secondDir, "main.cpp")},
	}
	for _, key := range []string{"main.cpp", "same"} {
		_, err := registry.Resolve(key)
		var ambiguous *AmbiguousError
		if !errors.As(err, &ambiguous) || len(ambiguous.Entries) != 2 {
			t.Fatalf("Resolve(%q) error = %v, want AmbiguousError", key, err)
		}
	}
}

func TestResolveReportsMissingAndStaleSources(t *testing.T) {
	root := t.TempDir()
	registry := Registry{
		567: {ID: 567, Name: "remote-only"},
		891: {ID: 891, Name: "stale", SourcePath: filepath.Join(root, "missing.cpp")},
	}
	if _, err := registry.Resolve("567"); !errors.Is(err, ErrNoLocalSource) {
		t.Fatalf("remote-only error = %v, want ErrNoLocalSource", err)
	}
	if _, err := registry.Resolve("891"); !errors.Is(err, ErrStaleSource) {
		t.Fatalf("stale error = %v, want ErrStaleSource", err)
	}
	if _, err := registry.Resolve("unknown"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown error = %v, want ErrNotFound", err)
	}
}

func createSource(t *testing.T, directory, filename string) string {
	t.Helper()
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, filename)
	if err := os.WriteFile(path, []byte("int main() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

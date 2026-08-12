package cli

import (
	"os"
	"path/filepath"
	"testing"

	"yoel/internal/graderapi"
	"yoel/internal/registry"
)

func TestRefreshQuestionRegistryMergesRemoteMetadataWithoutLocalPaths(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	workingDirectory := t.TempDir()
	sourcePath := filepath.Join(workingDirectory, "567.cpp")
	if err := os.WriteFile(sourcePath, []byte("int main() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := recordCreatedQuestion(graderapi.Problem{ID: 567, Name: "old", FullName: "Old Name"}, workingDirectory, sourcePath); err != nil {
		t.Fatal(err)
	}
	if err := refreshQuestionRegistry([]graderapi.Problem{{ID: 567, Name: "arrays", FullName: "Arrays"}, {ID: 891, Name: "vectors", FullName: "Vectors"}}); err != nil {
		t.Fatal(err)
	}

	questions, err := registry.LoadDefault()
	if err != nil {
		t.Fatal(err)
	}
	entry := questions[567]
	if entry.Name != "arrays" || entry.FullName != "Arrays" || entry.DirectoryPath != workingDirectory || entry.SourcePath != sourcePath {
		t.Fatalf("merged entry = %#v", entry)
	}
	if entry := questions[891]; entry.ID != 891 || entry.Name != "vectors" || entry.SourcePath != "" {
		t.Fatalf("new entry = %#v", entry)
	}
}

func TestRecordCreatedQuestionAllowsDirectoryOnlyBinding(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	directory := t.TempDir()
	problem := graderapi.Problem{ID: 567, Name: "attachment", FullName: "Attachment Problem"}
	if err := recordCreatedQuestion(problem, directory, ""); err != nil {
		t.Fatal(err)
	}
	questions, err := registry.LoadDefault()
	if err != nil {
		t.Fatal(err)
	}
	entry := questions[567]
	if entry.DirectoryPath != directory || entry.SourcePath != "" || entry.Name != "attachment" {
		t.Fatalf("entry = %#v", entry)
	}
}

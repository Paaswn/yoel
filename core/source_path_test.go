package core

import (
	"os"
	"path/filepath"
	"testing"
)

func TestQueryBySourcePath(t *testing.T) {
	dir := t.TempDir()
	reg, err := NewRegistryWithPath(filepath.Join(dir, "registry.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer reg.Close()
	source := filepath.Join(dir, "solution with spaces.cpp")
	if err := os.WriteFile(source, []byte("int main() {}"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := reg.QueryBySourcePath(source); err == nil {
		t.Fatal("unregistered source accepted")
	}
	for _, id := range []int{1, 2} {
		if err := reg.Upsert(ProblemLite{ID: id, CodeName: "problem"}); err != nil {
			t.Fatal(err)
		}
		if err := reg.SetProblemPath(id, source, dir); err != nil {
			t.Fatal(err)
		}
		if id == 1 {
			got, err := reg.QueryBySourcePath(source)
			if err != nil || got.ID != 1 {
				t.Fatalf("resolve: %+v, %v", got, err)
			}
			link := filepath.Join(dir, "alias.cpp")
			if err := os.Symlink(source, link); err == nil {
				got, err := reg.QueryBySourcePath(link)
				if err != nil || got.ID != 1 {
					t.Fatalf("symlink: %+v, %v", got, err)
				}
			}
		}
	}
	if _, err := reg.QueryBySourcePath(source); err == nil {
		t.Fatal("ambiguous source accepted")
	}
	if err := os.Remove(source); err != nil {
		t.Fatal(err)
	}
	if _, err := reg.QueryBySourcePath(source); err == nil {
		t.Fatal("missing source accepted")
	}
}

func TestSetLocalProblemPath(t *testing.T) {
	p := ProblemLite{}
	setLocalProblemPath(&p, "/problem/main.cpp", "/problem")
	if !p.IsOnLocal || p.SourcePath != "/problem/main.cpp" || p.DirectoryPath != "/problem" {
		t.Fatalf("incomplete local metadata: %+v", p)
	}
}

package core

import (
	"fmt"
	"os"
)

// QueryBySourcePath resolves an editor file without guessing from its filename.
// SameFile handles symlinks and platform-specific path casing.
func (r *Registry) QueryBySourcePath(path string) (ProblemLite, error) {
	info, err := os.Stat(path)
	if err != nil {
		return ProblemLite{}, err
	}
	problems, err := r.GetAllProblems()
	if err != nil {
		return ProblemLite{}, err
	}
	var found *ProblemLite
	for _, p := range problems {
		if !p.IsOnLocal || p.SourcePath == "" {
			continue
		}
		source, err := os.Stat(p.SourcePath)
		if err == nil && os.SameFile(info, source) {
			if found != nil {
				return ProblemLite{}, fmt.Errorf("multiple problems register source file %q; submit by ID instead", path)
			}
			match := p
			found = &match
		}
	}
	if found == nil {
		return ProblemLite{}, fmt.Errorf("no problem registers source file %q; open its registered source or run 'yoel new'", path)
	}
	return *found, nil
}

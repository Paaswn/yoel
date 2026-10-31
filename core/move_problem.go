package core

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/adrg/xdg"
)

type MoveOption struct {
    Trash bool
    Path string
}

var ProblemNotExistLocally = errors.New("the problem doesn't exist locally")
func MoveProblem(ctx context.Context, problem *ProblemLite, reg *Registry, options MoveOption) error {
    if (!problem.IsOnLocal) {
        return ProblemNotExistLocally
    }
    var newRootPath string
    var err error
    if options.Trash {
        newRootPath = filepath.Join( xdg.DataHome, Yoel, "trash" )
        if mkdirErr := os.MkdirAll(newRootPath, 0o755); mkdirErr != nil {
            return mkdirErr
        }
    } else {
        newRootPath, err = filepath.Abs(filepath.Clean(options.Path))
        if err != nil {
            return err
        }
    }
	newDirPath := filepath.Join(newRootPath, problem.CodeName)
    sourceName := filepath.Base(problem.SourcePath)
    if err = os.Rename(problem.DirectoryPath, newDirPath); err != nil {
        return err
    }
    newSourcePath := filepath.Join(newDirPath, sourceName)
    return reg.SetProblemPath(problem.ID, newSourcePath, newDirPath)
}
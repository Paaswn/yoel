package core

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	gapi "github.com/Paaswn/yoel/graderapi"
)

const yoelHiddenDir string = ".yoel"
func CreateQuestion(ctx context.Context, session SavedSession, problem ProblemLite, reg *Registry) error {
    client, err := gapi.NewClient(DefaultGraderURL, nil)
    if err != nil {
        return err
    }
    cwd, err := os.Getwd()
    if err != nil {
        return err
    }
    return createQuestion(ctx, cwd, problem, client.WithToken(session.Token), reg)
}

func createQuestion(ctx context.Context, cwd string, problem ProblemLite, client *gapi.Client, reg *Registry) error {
    c, cancel := context.WithTimeout(ctx, TimeOut)
    defer cancel()
    rawPDF, err := client.DownloadProblemPDF(c, problem.ID)
    if err != nil {
        return err
    }
    id := problem.ID
    temp, err := os.MkdirTemp(cwd, "yoel-temp-*" )
    defer os.RemoveAll(temp)
    if err != nil {
        return err
    }
    pdfPath := filepath.Join(temp, strconv.Itoa(id)+".pdf")
    if err = os.WriteFile(pdfPath, rawPDF.Data, 0o444); err != nil {
        return err
    }
    var source string
    if problem.HasAttachment {
        attachment, err := getAttachment(ctx, client, id)
        if err != nil {
            return err
        }
        res_source, err := extractQuestionIntoDir(temp, attachment)
        if err != nil {
            return err
        }
        source = res_source
    } else {
        err := makeEmptySourceFile(temp)
        if err != nil {
            return err
        }
        source = "main.cpp"
    }
    hidDir := filepath.Join(temp, yoelHiddenDir)
    if err = os.Mkdir(hidDir, 0o755);err != nil {
        return err
    }
    dirPath:= filepath.Join(cwd, problem.CodeName)
    sourcePath := filepath.Join(dirPath, source)
    if err := os.Rename(temp, dirPath); err != nil {
        return err
    }
    if err := reg.SetProblemPath(problem.ID, sourcePath, dirPath); err != nil {
        return err
    }
    return nil
}

func extractQuestionIntoDir(dir string, attachment gapi.ProblemFile) (string, error ) {
	archive, err := zip.NewReader(bytes.NewReader(attachment.Data), int64(len(attachment.Data)))
	if err != nil {
		return "", err
	}
	var sourceName string
	for _, entry := range archive.File {
    	target := filepath.Join(dir, entry.Name)
        rel, err := filepath.Rel(dir, target)
        if err != nil {
            return "", err
        }
        if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
            return "", fmt.Errorf("unsafe attachment path: %q", entry.Name)
        }
    	if entry.FileInfo().IsDir() {
    		if err := os.MkdirAll(target, 0o755); err != nil {
    			return "", err
    		}
    		continue
    	}
        if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
 			return "", err
  		}
        reader, err := entry.Open()
        if err != nil {
			return "", err
		}
		if validSourceName(target, sourceName) {
            sourceName, err = filepath.Rel(dir, target)
            if err != nil {
                return "", err
            }
  		}
  		file, err := os.Create(target)
        if err != nil {
            reader.Close()
            return "", err
        }

        _, copyErr := io.Copy(file, reader)

        readerErr := reader.Close()
        fileErr := file.Close()

        if copyErr != nil {
            return "", copyErr
        }
        if readerErr != nil {
            return "", readerErr
        }
        if fileErr != nil {
            return "", fileErr
        }
	}

    return sourceName, nil
}

func validSourceName(target, sourceName string) bool {
    targetBase := filepath.Base(target)
    sourceBase := filepath.Base(sourceName)
    return targetBase == "main.h" && sourceBase != "student.h" ||
            targetBase == "student.h" ||
            sourceBase != "student.h"  && sourceBase != "main.h" && strings.Contains(targetBase, ".cpp")
}
const yoelSourceFile =
`/*
--- this file was automatically created by yoel ---
*/

#include <iostream>
using namespace std;

int main() {

}
`
func makeEmptySourceFile(dir string) error {
    sourceFile := filepath.Join(dir, "main.cpp")
    if err := os.WriteFile(sourceFile, []byte(yoelSourceFile), 0o755); err != nil {
        return err
    }

    return nil
}

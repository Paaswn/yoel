package core

import (
	"context"
	"os"
	"path/filepath"
	"strconv"

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
    if problem.HasAttachment {
        attachment, err := getAttachment(ctx, client, id)
        if err != nil {
            return err
        }
        if err = extractQuestionIntoDir(temp, attachment); err != nil {
            return err
        }
    } else {
        if err = makeEmptySourceFile(temp); err != nil {
            return err
        }
    }
    hidDir := filepath.Join(temp, yoelHiddenDir)
    if err = os.Mkdir(hidDir, 0o755);err != nil {
        return err
    }
    truepath:= filepath.Join(cwd, problem.CodeName)
    reg.SetProblemPath(problem.ID, "", truepath)
    return os.Rename(temp, truepath)
}

func extractQuestionIntoDir(dir string, attachment gapi.ProblemFile) error {
    return nil
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
    return os.WriteFile(sourceFile, []byte(yoelSourceFile), 0o755);
}

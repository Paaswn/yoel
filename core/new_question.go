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
    idPath := filepath.Join(temp, strconv.Itoa(id)+".id")
    if err = os.WriteFile(idPath, []byte(""), 0o000); err != nil {
        return err
    }
    pdfPath := filepath.Join(temp, rawPDF.Filename)
    if err = os.WriteFile(pdfPath, rawPDF.Data, 0o444); err != nil {
        return err
    }
    hidDir := filepath.Join(temp, yoelHiddenDir)
    if err = os.Mkdir(hidDir, 0o755);err != nil {
        return err
    }
    truepath:= filepath.Join(cwd, problem.CodeName)
    reg.SetProblemPath(problem.ID, "", truepath)
    return os.Rename(temp, truepath)
}
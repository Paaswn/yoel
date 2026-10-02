package core

import (
	"context"
	"os"
	"path/filepath"
	"strconv"

	gapi "github.com/Paaswn/yoel/graderapi"
)

const yoelHiddenDir string = ".yoel"
func CreateQuestion(ctx context.Context, session SavedSession, problem ProblemLite) error {
    client, err := gapi.NewClient(DefaultGraderURL, nil)
    if err != nil {
        return err
    }
    rawPDF, err := client.WithToken(session.Token).DownloadProblemPDF(ctx, problem.ID)
    if err != nil {
        return err
    }
    return createQuestionWithPDF(problem, rawPDF)
}

func createQuestionWithPDF(problem ProblemLite, rawPDF gapi.ProblemFile) error {
    cwd, err := os.Getwd()
    id := problem.ID
    if err != nil {
        return err
    }
    temp, err := os.MkdirTemp(cwd, "yoel-temp-*" )
    defer os.RemoveAll(temp)
    if err != nil {
        return err
    }
    idPath := filepath.Join(temp, strconv.Itoa(id)+".id")
    os.WriteFile(idPath, []byte(""), 0o544)
    
    pdfPath := filepath.Join(temp, rawPDF.Filename)
    err = os.WriteFile(pdfPath, rawPDF.Data, 0o644)
    if err != nil {
        return err
    }
    hidDir := filepath.Join(temp, yoelHiddenDir)
    if err = os.Mkdir(hidDir, 0o755);err != nil {
        return err
    }
    truepath:= filepath.Join(cwd, problem.CodeName)
    return os.Rename(temp, truepath)
}
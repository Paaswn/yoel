package core

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	gapi "github.com/Paaswn/yoel/graderapi"
)

const yoelHiddenDir string = ".yoel"

type CreateProblemArgs struct {
	Ctx     context.Context
	Session SavedSession
	Problem *ProblemLite
	Reg     *Registry
	Force   bool
}

func CreateProblem(args CreateProblemArgs) error {
	client, err := gapi.NewClient(DefaultGraderURL, nil)
	if err != nil {
		return err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	return createProblem(args.Ctx, cwd, args.Problem,  client.WithToken(args.Session.Token), args.Reg, args.Force)
}
var ProblemExistErr = errors.New("problem path already exists")
func createProblem(ctx context.Context, cwd string, problem *ProblemLite, client *gapi.Client, reg *Registry, force bool) error {
	c, cancel := context.WithTimeout(ctx, TimeOut)
	defer cancel()
	rawPDF, err := client.DownloadProblemPDF(c, problem.ID)
	if err != nil {
		return err
	}
	id := problem.ID
	temp, err := os.MkdirTemp(cwd, "yoel-temp-*")
	defer os.RemoveAll(temp)
	if err != nil {
		return err
	}
	pdfName := strconv.Itoa(id) + ".pdf"
	pdfPath := filepath.Join(temp, pdfName)
	if err = os.WriteFile(pdfPath, rawPDF.Data, 0o444); err != nil {
		return err
	}
	var source string
	if problem.HasAttachment {
		attachment, err := getAttachment(ctx, client, id)
		if err != nil {
			return err
		}
		res_source, err := extractProblemIntoDir(temp, attachment)
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
	if err = os.Mkdir(hidDir, 0o755); err != nil {
		return err
	}
	dirPath := filepath.Join(cwd, problem.CodeName)
	sourcePath := filepath.Join(dirPath, source)

	if _, statErr := os.Stat(dirPath); statErr == nil {
		if !force {
			return ProblemExistErr
		}

		if err := os.RemoveAll(dirPath); err != nil {
			return err
		}
	} else if !os.IsNotExist(statErr) {
		return statErr
	}

	
	if err := os.Rename(temp, dirPath); err != nil {
		return err
	}
	err = openWithDefaultViewer(filepath.Join(dirPath, pdfName))
	if err != nil {
		return err
	}
	if err := reg.SetProblemPath(problem.ID, sourcePath, dirPath); err != nil {
		return err
	}
	setLocalProblemPath(problem, sourcePath, dirPath)
	return nil
}

func setLocalProblemPath(problem *ProblemLite, sourcePath string, dirPath string) {
	problem.DirectoryPath = dirPath
	problem.SourcePath = sourcePath
}

var yoelBanner = `/*
--- this file was prepared by yoel ---
*/
`

func extractProblemIntoDir(dir string, attachment gapi.ProblemFile) (string, error) {
	archive, err := zip.NewReader(bytes.NewReader(attachment.Data), int64(len(attachment.Data)))
	if err != nil {
		if errors.Is(err, zip.ErrFormat) {
			// extremely edge cases from the unpredictable cafe-grader rev 4.7.3cu's API
			if filepath.Ext(attachment.Filename) != ".cpp" {
				return "", fmt.Errorf("file extension is neither zip nor cpp: %w", err)
			}
			attachmentPath := filepath.Join(dir, attachment.Filename)
			if err = os.WriteFile(attachmentPath, attachment.Data, 0o755); err != nil {
				return "", err
			}
			return attachment.Filename, nil
		}
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
		file, err := os.Create(target)
		if err != nil {
			reader.Close()
			return "", err
		}
		if validSourceName(target, sourceName) {
			sourceName, err = filepath.Rel(dir, target)
			if err != nil {
				return "", err
			}
			if _, err := io.WriteString(file, yoelBanner); err != nil {
				reader.Close()
				file.Close()
				return "", err
			}
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
		sourceBase != "student.h" && sourceBase != "main.h" && strings.Contains(targetBase, ".cpp")
}

const yoelSourceFile = `/*
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

func openWithDefaultViewer(path string) error {
	var command *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		command = exec.Command("open", path)
	case "windows":
		command = exec.Command("rundll32", "url.dll,FileProtocolHandler", filepath.Clean(path))
	default:
		command = exec.Command("xdg-open", path)
	}
	if err := command.Start(); err != nil {
		return fmt.Errorf("open PDF with default viewer: %w", err)
	}
	return nil
}
func getAttachment(ctx context.Context, client *gapi.Client, id int) (gapi.ProblemFile, error) {
	attachment, err := client.DownloadProblemAttachment(ctx, id)
	if err != nil {
		return gapi.ProblemFile{}, err
	}
	return attachment, nil
}

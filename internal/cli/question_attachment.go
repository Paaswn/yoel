package cli

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"

	"yoel/internal/graderapi"
)

const (
	maxExtractedAttachmentSize int64 = 128 << 20
	maxExtractedFileSize       int64 = 32 << 20
	maxAttachmentEntries             = 4096
	maxAttachmentFiles               = 1024
	maxAttachmentDepth               = 32
	maxAttachmentPathLength          = 240
)

func createQuestionFromAttachment(ctx context.Context, session storedSession, problem graderapi.Problem, temporaryDir string) (bool, error) {
	// problemDir, err := questionDirectory(currentDir, problem.Name)
	// if err != nil {
	// 	return err
	// }
	// if _, err := os.Lstat(problemDir); err == nil {
	// 	return fmt.Errorf("create question directory: %s already exists", problemDir)
	// } else if !os.IsNotExist(err) {
	// 	return fmt.Errorf("inspect question directory: %w", err)
	// }
	// using
	client, err := graderapi.NewClient(session.BaseURL, nil)
	if err != nil {
		return false, fmt.Errorf("question attachment: %w", err)
	}
	attachment, err := client.WithToken(session.Token).DownloadProblemAttachment(ctx, problem.ID)
	if err != nil {
		return false, err
	}
	// using
	// temporaryDir, err := os.MkdirTemp(currentDir, ".yoel-question-*")
	// if err != nil {
	// 	return fmt.Errorf("create temporary question directory: %w", err)
	// }
	// defer os.RemoveAll(temporaryDir)

	files, err := extractQuestionAttachment(attachment.Data, temporaryDir)
	if err != nil {
		return false, err
	}
	if len(files) == 1 {
		target := filepath.Join(temporaryDir, strconv.Itoa(problem.ID)+".cpp")
		if err := renameSingleAttachmentSource(files[0], target, temporaryDir); err != nil {
			return false, err
		}
		if err := removeReadPDFConflicts(temporaryDir); err != nil {
			return false, err
		}
	}
	// using
	// if err := createQuestionPDFReference(temporaryDir, problemDir, pdfPath); err != nil {
	// 	return err
	// }
	// if err := os.Rename(temporaryDir, problemDir); err != nil {
	// 	return fmt.Errorf("create question directory: %w", err)
	// }
	return len(files) == 1, nil
}

func questionDirectory(currentDir, problemName string) (string, error) {
	invalid := problemName == "" || problemName == "." || problemName == ".." ||
		filepath.IsAbs(problemName) || filepath.VolumeName(problemName) != "" ||
		strings.ContainsAny(problemName, `/\`) || invalidWindowsPathComponent(problemName)
	if invalid {
		return "", fmt.Errorf("create question directory: invalid problem name %q", problemName)
	}
	return filepath.Join(currentDir, problemName), nil
}

func extractQuestionAttachment(data []byte, destination string) ([]string, error) {
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("extract question attachment: invalid ZIP archive: %w", err)
	}

	if err := validateQuestionAttachment(archive, destination); err != nil {
		return nil, err
	}

	files := make([]string, 0, len(archive.File))
	var extractedSize int64
	for _, entry := range archive.File {
		target, err := safeAttachmentPath(destination, entry.Name)
		if err != nil {
			return nil, err
		}
		if entry.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return nil, fmt.Errorf("extract question attachment: create directory: %w", err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return nil, fmt.Errorf("extract question attachment: create parent directory: %w", err)
		}
		reader, err := entry.Open()
		if err != nil {
			return nil, fmt.Errorf("extract question attachment: open %q: %w", entry.Name, err)
		}
		file, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			reader.Close()
			return nil, fmt.Errorf("extract question attachment: create %q: %w", entry.Name, err)
		}
		remaining := min(maxExtractedAttachmentSize-extractedSize, maxExtractedFileSize)
		written, copyErr := io.Copy(file, io.LimitReader(reader, remaining+1))
		closeErr := file.Close()
		readerErr := reader.Close()
		extractedSize += written
		if copyErr != nil {
			return nil, fmt.Errorf("extract question attachment: write %q: %w", entry.Name, copyErr)
		}
		if closeErr != nil {
			return nil, fmt.Errorf("extract question attachment: close %q: %w", entry.Name, closeErr)
		}
		if readerErr != nil {
			return nil, fmt.Errorf("extract question attachment: read %q: %w", entry.Name, readerErr)
		}
		if written > maxExtractedFileSize {
			return nil, fmt.Errorf("extract question attachment: file %q exceeds %d bytes", entry.Name, maxExtractedFileSize)
		}
		if extractedSize > maxExtractedAttachmentSize {
			return nil, fmt.Errorf("extract question attachment: expanded archive exceeds %d bytes", maxExtractedAttachmentSize)
		}
		files = append(files, target)
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("extract question attachment: archive contains no files")
	}
	return files, nil
}

func validateQuestionAttachment(archive *zip.Reader, destination string) error {
	if len(archive.File) > maxAttachmentEntries {
		return fmt.Errorf("extract question attachment: archive contains more than %d entries", maxAttachmentEntries)
	}

	seen := make(map[string]bool, len(archive.File))
	var declaredSize uint64
	fileCount := 0
	for _, entry := range archive.File {
		if !entry.FileInfo().IsDir() && entry.Mode().IsRegular() {
			fileCount++
		}
	}
	if fileCount > maxAttachmentFiles {
		return fmt.Errorf("extract question attachment: archive contains more than %d files", maxAttachmentFiles)
	}
	for _, entry := range archive.File {
		if _, err := safeAttachmentPath(destination, entry.Name); err != nil {
			return err
		}
		cleaned, err := cleanAttachmentEntryName(entry.Name)
		if err != nil {
			return err
		}
		if entry.Mode()&os.ModeSymlink != 0 || (!entry.FileInfo().IsDir() && !entry.Mode().IsRegular()) {
			return fmt.Errorf("extract question attachment: unsupported entry %q", entry.Name)
		}
		isDirectory := entry.FileInfo().IsDir()
		canonical := strings.ToLower(cleaned)
		if (canonical == "read.pdf" || strings.HasPrefix(canonical, "read.pdf/")) && fileCount != 1 {
			return fmt.Errorf("extract question attachment: entry %q conflicts with Read.pdf", entry.Name)
		}
		if _, exists := seen[canonical]; exists {
			return fmt.Errorf("extract question attachment: duplicate path %q", entry.Name)
		}
		parts := strings.Split(canonical, "/")
		for i := 1; i < len(parts); i++ {
			if parentIsDirectory, exists := seen[strings.Join(parts[:i], "/")]; exists && !parentIsDirectory {
				return fmt.Errorf("extract question attachment: path %q is inside a file", entry.Name)
			}
		}
		if !isDirectory {
			for existing := range seen {
				if strings.HasPrefix(existing, canonical+"/") {
					return fmt.Errorf("extract question attachment: file %q conflicts with a directory", entry.Name)
				}
			}
		}
		seen[canonical] = isDirectory

		entrySize := entry.UncompressedSize64
		if entrySize > uint64(maxExtractedFileSize) {
			return fmt.Errorf("extract question attachment: file %q exceeds %d bytes", entry.Name, maxExtractedFileSize)
		}
		if entrySize > uint64(maxExtractedAttachmentSize) || declaredSize > uint64(maxExtractedAttachmentSize)-entrySize {
			return fmt.Errorf("extract question attachment: expanded archive exceeds %d bytes", maxExtractedAttachmentSize)
		}
		declaredSize += entrySize
	}
	return nil
}

func cleanAttachmentEntryName(entryName string) (string, error) {
	normalized := strings.ReplaceAll(entryName, `\`, "/")
	cleaned := path.Clean(normalized)
	if cleaned == "." || path.IsAbs(cleaned) || cleaned == ".." || strings.HasPrefix(cleaned, "../") || len(cleaned) > maxAttachmentPathLength {
		return "", fmt.Errorf("extract question attachment: unsafe path %q", entryName)
	}
	parts := strings.Split(cleaned, "/")
	if len(parts) > maxAttachmentDepth {
		return "", fmt.Errorf("extract question attachment: path %q exceeds maximum depth", entryName)
	}
	for _, part := range parts {
		if invalidWindowsPathComponent(part) {
			return "", fmt.Errorf("extract question attachment: unsafe path %q", entryName)
		}
	}
	return cleaned, nil
}

func invalidWindowsPathComponent(component string) bool {
	if component == "" || strings.Contains(component, ":") || strings.IndexFunc(component, unicode.IsControl) >= 0 || strings.HasSuffix(component, " ") || strings.HasSuffix(component, ".") || looksLikeWindowsShortName(component) {
		return true
	}
	stem := strings.ToUpper(strings.SplitN(component, ".", 2)[0])
	if stem == "CON" || stem == "PRN" || stem == "AUX" || stem == "NUL" {
		return true
	}
	return len(stem) == 4 && (strings.HasPrefix(stem, "COM") || strings.HasPrefix(stem, "LPT")) && stem[3] >= '1' && stem[3] <= '9'
}

func looksLikeWindowsShortName(component string) bool {
	for index := 0; index+1 < len(component); index++ {
		if component[index] == '~' && component[index+1] >= '1' && component[index+1] <= '9' {
			return true
		}
	}
	return false
}

func safeAttachmentPath(destination, entryName string) (string, error) {
	cleaned, err := cleanAttachmentEntryName(entryName)
	if err != nil {
		return "", err
	}
	target := filepath.Join(destination, filepath.FromSlash(cleaned))
	relative, err := filepath.Rel(destination, target)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("extract question attachment: unsafe path %q", entryName)
	}
	return target, nil
}

func renameSingleAttachmentSource(source, target, root string) error {
	source = filepath.Clean(source)
	target = filepath.Clean(target)
	originalSource := source
	if source == target {
		return nil
	}

	if targetInfo, err := os.Lstat(target); err == nil {
		sourceInfo, sourceErr := os.Lstat(source)
		if sourceErr != nil {
			return fmt.Errorf("inspect attachment source: %w", sourceErr)
		}
		if os.SameFile(sourceInfo, targetInfo) {
			temporaryFile, err := os.CreateTemp(root, ".yoel-source-rename-*")
			if err != nil {
				return fmt.Errorf("prepare attachment source rename: %w", err)
			}
			intermediate := temporaryFile.Name()
			if err := temporaryFile.Close(); err != nil {
				os.Remove(intermediate)
				return fmt.Errorf("prepare attachment source rename: %w", err)
			}
			if err := os.Remove(intermediate); err != nil {
				return fmt.Errorf("prepare attachment source rename: %w", err)
			}
			if err := os.Rename(source, intermediate); err != nil {
				return fmt.Errorf("rename attachment source: %w", err)
			}
			if err := os.Rename(intermediate, target); err != nil {
				_ = os.Rename(intermediate, source)
				return fmt.Errorf("rename attachment source: %w", err)
			}
			removeEmptyParents(filepath.Dir(source), root)
			return nil
		}
		if !targetInfo.IsDir() {
			return fmt.Errorf("rename attachment source: target %s already exists", target)
		}
		if pathWithin(source, target) {
			temporaryFile, err := os.CreateTemp(root, ".yoel-source-move-*")
			if err != nil {
				return fmt.Errorf("prepare attachment source move: %w", err)
			}
			intermediate := temporaryFile.Name()
			if err := temporaryFile.Close(); err != nil {
				os.Remove(intermediate)
				return fmt.Errorf("prepare attachment source move: %w", err)
			}
			if err := os.Remove(intermediate); err != nil {
				return fmt.Errorf("prepare attachment source move: %w", err)
			}
			if err := os.Rename(source, intermediate); err != nil {
				return fmt.Errorf("move attachment source: %w", err)
			}
			source = intermediate
		}
		if err := os.RemoveAll(target); err != nil {
			return fmt.Errorf("remove empty attachment directories: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect attachment source target: %w", err)
	}

	if err := os.Rename(source, target); err != nil {
		return fmt.Errorf("rename attachment source: %w", err)
	}
	removeEmptyParents(filepath.Dir(originalSource), root)
	return nil
}

func pathWithin(child, parent string) bool {
	relative, err := filepath.Rel(parent, child)
	return err == nil && relative != "." && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func removeReadPDFConflicts(root string) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		return fmt.Errorf("inspect extracted question files: %w", err)
	}
	for _, entry := range entries {
		if strings.EqualFold(entry.Name(), "Read.pdf") {
			if err := os.RemoveAll(filepath.Join(root, entry.Name())); err != nil {
				return fmt.Errorf("prepare Read.pdf: %w", err)
			}
		}
	}
	return nil
}

func createQuestionPDFReference(temporaryDir, finalDir, pdfPath string) error {
	referencePath := filepath.Join(temporaryDir, "Read.pdf")
	if relativeTarget, err := filepath.Rel(finalDir, pdfPath); err == nil {
		if err := os.Symlink(relativeTarget, referencePath); err == nil {
			return nil
		}
	}

	source, err := os.Open(pdfPath)
	if err != nil {
		return fmt.Errorf("open cached statement PDF: %w", err)
	}
	defer source.Close()
	destination, err := os.OpenFile(referencePath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return fmt.Errorf("create Read.pdf: %w", err)
	}
	_, copyErr := io.Copy(destination, source)
	closeErr := destination.Close()
	if copyErr != nil {
		return fmt.Errorf("copy statement PDF: %w", copyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close Read.pdf: %w", closeErr)
	}
	return nil
}

func removeEmptyParents(start, stop string) {
	stop = filepath.Clean(stop)
	for current := filepath.Clean(start); current != stop; current = filepath.Dir(current) {
		if err := os.Remove(current); err != nil {
			return
		}
	}
}

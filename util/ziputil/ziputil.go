package ziputil

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func ZipFolder(source, targetZip string) error {
	zipFile, err := os.Create(targetZip)
	if err != nil {
		return err
	}
	defer zipFile.Close()

	writer := zip.NewWriter(zipFile)
	defer writer.Close()

	return filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relPath, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if relPath == "." {
			return nil
		}
		if info.IsDir() {
			_, err := writer.Create(strings.ReplaceAll(relPath, "\\", "/") + "/")
			return err
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		zipWriter, err := writer.Create(strings.ReplaceAll(relPath, "\\", "/"))
		if err != nil {
			return err
		}
		_, err = io.Copy(zipWriter, f)
		return err
	})
}

func UnzipFolder(sourceZip, targetDir string) error {
	r, err := zip.OpenReader(sourceZip)
	if err != nil {
		return err
	}
	defer r.Close()
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return err
	}
	for _, file := range r.File {
		extractedPath := filepath.Join(targetDir, file.Name)
		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(extractedPath, 0700); err != nil {
				return err
			}
			continue
		}
		if err := extractFile(file, extractedPath); err != nil {
			return err
		}
	}
	return nil
}

func extractFile(file *zip.File, outputPath string) error {
	rc, err := file.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	outputFile, err := os.OpenFile(outputPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, file.Mode())
	if err != nil {
		return err
	}
	defer outputFile.Close()
	_, err = io.Copy(outputFile, rc)
	return err
}

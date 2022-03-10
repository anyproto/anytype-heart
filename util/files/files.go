package files

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// WriteReaderIntoFileReuseSameExistingFile has simple logic
// 1. if path not exists the reader(r) just copied to the file in the provided path
// 2. if path exists and the content is equals to reader(r) the existing path is returning
// 3. if path exists but the content is not equals reader(r), the path with random suffix is returning
func WriteReaderIntoFileReuseSameExistingFile(path string, r io.Reader) (string, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		f, err := os.Create(path)
		if err != nil {
			return "", err
		}
		_, err = io.Copy(f, r)
		if err != nil {
			return "", err
		}

		return path, f.Close()
	}

	var (
		ext  = filepath.Ext(path)
		dir  = filepath.Dir(path)
		name = strings.TrimSuffix(filepath.Base(path), ext)
	)

	if name == "." {
		name = "file"
	}

	tmp, err := os.CreateTemp(dir, name+"-*"+ext)
	_, err = io.Copy(tmp, r)
	if err != nil {
		return "", err
	}

	if t, err := AreFilesEqual(tmp.Name(), path); err == nil && t {
		tmpPath := tmp.Name()
		_ = tmp.Truncate(0)
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		// return path for existing file instead
		return path, nil
	}

	return tmp.Name(), tmp.Close()
}

func AreFilesEqual(file1, file2 string) (bool, error) {
	const chunkSize = 64000
	f1s, err := os.Stat(file1)
	if err != nil {
		return false, err
	}
	f2s, err := os.Stat(file2)
	if err != nil {
		return false, err
	}

	/* may return false-positive on windows if size and name are the same
	if os.SameFile(f1s, f2s) {
		return true, nil
	}*/

	if f1s.Size() != f2s.Size() {
		return false, nil
	}

	f1, err := os.Open(file1)
	if err != nil {
		return false, err
	}

	f2, err := os.Open(file2)
	if err != nil {
		return false, err
	}

	for {
		b1 := make([]byte, chunkSize)
		_, err1 := f1.Read(b1)

		b2 := make([]byte, chunkSize)
		_, err2 := f2.Read(b2)

		if err1 != nil || err2 != nil {
			if err1 == io.EOF && err2 == io.EOF {
				return true, nil
			} else if err1 == io.EOF && err2 == io.EOF {
				return false, nil
			} else {
				return false, err
			}
		}

		if !bytes.Equal(b1, b2) {
			return false, nil
		}
	}
}

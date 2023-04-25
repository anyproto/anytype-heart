package main

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
)

const (
	argsLenWithFile  = 2
	fileArgNumber    = 1
	stdinSizeOnLinux = 4092
	bufferSize       = 1024
)

func main() {
	var (
		err    error
		reader io.Reader
		file   = os.Stdin
	)
	if len(os.Args) == argsLenWithFile {
		if file, err = os.Open(os.Args[fileArgNumber]); err != nil {
			fmt.Println("Error opening file:", err)
			return
		}
	} else {
		fmt.Print("Enter base64 encoded string: ")
	}
	scanner := bufio.NewScanner(file)
	scanner.Scan()
	base64Str := scanner.Text()

	gzipBytes, err := base64.StdEncoding.DecodeString(base64Str)
	if err != nil {
		fmt.Println("Error decoding base64:", err)
		// On some OS stdin is limited with 4092 bytes
		if errors.Is(err, base64.CorruptInputError(stdinSizeOnLinux)) {
			fmt.Println("Try to pass base64 in a file. Filename should be the argument of the program")
		}
		return
	}

	br := bytes.NewReader(gzipBytes)
	zr, err := gzip.NewReader(br)
	if err != nil {
		reader = br
	} else {
		reader = zr
		defer zr.Close()
	}

	result := ""
	buf := make([]byte, bufferSize)
	for {
		n, err := reader.Read(buf)
		result += string(buf[:n])
		if err != nil {
			break
		}
	}

	fmt.Println(result)
}

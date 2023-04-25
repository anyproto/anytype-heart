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

func main() {
	var (
		err    error
		reader io.Reader
		file   = os.Stdin
	)
	if len(os.Args) == 2 {
		if file, err = os.Open(os.Args[1]); err != nil {
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
		if errors.Is(err, base64.CorruptInputError(4092)) {
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
	buf := make([]byte, 1024)
	for {
		n, err := reader.Read(buf)
		result += string(buf[:n])
		if err != nil {
			break
		}
	}

	fmt.Println(result)
}

package main

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"fmt"
	"os"
)

func main() {
	scanner := bufio.NewScanner(os.Stdin)
	fmt.Print("Enter base64 encoded string: ")
	scanner.Scan()
	base64Str := scanner.Text()

	gzipBytes, err := base64.StdEncoding.DecodeString(base64Str)
	if err != nil {
		fmt.Println("Error decoding base64:", err)
		return
	}

	reader, err := gzip.NewReader(bytes.NewReader(gzipBytes))
	if err != nil {
		fmt.Println("Error creating gzip reader:", err)
		return
	}
	defer reader.Close()

	result := ""
	buf := make([]byte, 1024)
	for {
		n, err := reader.Read(buf)
		if err != nil {
			break
		}
		result += string(buf[:n])
	}

	fmt.Println(result)
}

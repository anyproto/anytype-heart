package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/gogo/protobuf/jsonpb"
	"github.com/gogo/protobuf/proto"

	"github.com/anyproto/anytype-heart/pb"
)

func run() error {
	if len(os.Args) == 1 {
		return fmt.Errorf("select command: generate-json-helpers")
	}

	if os.Args[1] == "generate-json-helpers" {
		return generateJsonHelpers()
	}
	return nil
}

func generateJsonHelpers() error {
	rootPath := "./tests/integration/testdata/import"
	entries, err := os.ReadDir(rootPath)
	if err != nil {
		return fmt.Errorf("read dir: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			err := generateJsonHelpersForImportCase(filepath.Join(rootPath, entry.Name()))
			if err != nil {
				return fmt.Errorf("generate json helpers for dir %s: %w", entry.Name(), err)
			}
		}
	}
	return nil
}

func generateJsonHelpersForImportCase(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read dir: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		// Remove old json files
		if filepath.Ext(entry.Name()) == ".txt" {
			path := filepath.Join(dir, entry.Name())
			fmt.Println("delete old json file: ", path)
			err := os.Remove(path)
			if err != nil {
				return fmt.Errorf("remove file: %w", err)
			}
		}
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if filepath.Ext(entry.Name()) == ".pb" {
			err = generateJsonHelper(dir, entry.Name())
			if err != nil {
				return fmt.Errorf("generate helper: %w", err)
			}
		}
	}
	return nil
}

func generateJsonHelper(dir string, pbFileName string) error {
	f, err := os.Open(filepath.Join(dir, pbFileName))
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer f.Close()
	snapshot := &pb.SnapshotWithType{}

	data, err := io.ReadAll(f)
	if err != nil {
		return fmt.Errorf("read pb file: %w", err)
	}
	err = proto.Unmarshal(data, snapshot)
	if err != nil {
		return fmt.Errorf("unmarshal pb: %w", err)
	}

	jsonFilePath := filepath.Join(dir, pbFileName+".txt")
	jsonFile, err := os.Create(jsonFilePath)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	defer jsonFile.Close()

	marshaler := &jsonpb.Marshaler{Indent: "  "}
	err = marshaler.Marshal(jsonFile, snapshot)
	if err != nil {
		return fmt.Errorf("marshal to json: %w", err)
	}
	fmt.Println("created json file: ", jsonFilePath)
	return nil
}

func main() {
	err := run()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

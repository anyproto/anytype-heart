package main

import (
	"archive/zip"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/gogo/protobuf/jsonpb"
	"github.com/gogo/protobuf/proto"

	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/util/constant"
)

var (
	jsonM = jsonpb.Marshaler{Indent: "  "}
)

func main() {
	// command line flags
	unpack := flag.String("unpack", "", "Unpack zip archive with pb files to the directory with json files")
	pack := flag.String("pack", "", "Convert json files in a directory to pb and write to a zip file")

	flag.Parse()

	// check input and output are specified
	if (*pack == "" && *unpack == "") || (*pack != "" && *unpack != "") {
		log.Fatalf("You should specify either -pack or -unpack")
	}

	if *pack != "" {
		createZipFromDirectory(*pack, *pack+".zip")
		return
	}

	if *unpack != "" {
		output := strings.TrimSuffix(*unpack, filepath.Ext(*unpack))
		// check if dir exists and create if not
		if _, err := os.Stat(output); os.IsNotExist(err) {
			if err := os.MkdirAll(output, 0755); err != nil {
				log.Fatalf("Failed to create output directory: %v", err)
			}
		} else {
			log.Fatalf("Output directory already exists: %v", output)
		}

		handleZip(*unpack, output)
	}
}

type File struct {
	Name string
	RC   io.ReadCloser
}

func processFile(file File, outputFile string) {
	defer file.RC.Close()

	if !strings.HasSuffix(file.Name, ".pb") && !strings.HasPrefix(file.Name, constant.ProfileFile) {
		// write file as is
		if err := os.MkdirAll(filepath.Dir(outputFile), 0755); err != nil {
			log.Fatalf("Failed to create output subdirectory: %v", err)
		}
		// write from reader to file
		outFile, err := os.Create(outputFile)
		if err != nil {
			log.Fatalf("Failed to create output file: %v", err)
		}
		defer outFile.Close()
		if _, err := io.Copy(outFile, file.RC); err != nil {
			log.Fatalf("Failed to copy file: %v", err)
		}
		return
	}
	// assuming Snapshot is a protobuf message
	var snapshot proto.Message = &pb.ChangeSnapshot{}
	if strings.HasPrefix(file.Name, constant.ProfileFile) {
		snapshot = &pb.Profile{}
	}

	content, err := ioutil.ReadAll(file.RC)
	if err != nil {
		log.Fatalf("Failed to read file: %v", err)
	}

	if err := proto.Unmarshal(content, snapshot); err != nil {
		snapshot = &pb.SnapshotWithType{}
		if err := proto.Unmarshal(content, snapshot); err != nil {
			log.Fatalf("Failed to parse protobuf message: %v", err)
		}
	}

	// convert to jsonpb and write to outputFile
	jsonData, err := jsonM.MarshalToString(snapshot)
	if err != nil {
		log.Fatalf("Failed to marshal protobuf message to json: %v", err)
	}

	outputFile = strings.TrimSuffix(outputFile, filepath.Ext(outputFile)) + ".json"
	if err := ioutil.WriteFile(outputFile, []byte(jsonData), 0644); err != nil {
		log.Fatalf("Failed to write json file: %v", err)
	}

}

func handleZip(input, output string) {
	r, err := zip.OpenReader(input)
	if err != nil {
		log.Fatalf("Failed to open zip: %v", err)
	}
	defer r.Close()

	for _, f := range r.File {
		dir := filepath.Dir(f.Name)
		if dir != "." {
			// nolint: gosec
			outputDir := filepath.Join(output, dir)
			if _, err := os.Stat(outputDir); os.IsNotExist(err) {
				if err := os.MkdirAll(outputDir, 0755); err != nil {
					log.Printf("Failed to create output subdirectory: %v\n", err)
					return
				}
			}
		}

		// assuming we are only working with files, not directories
		if f.FileInfo().IsDir() {
			continue
		}

		rc, err := f.Open()
		if err != nil {
			fmt.Printf("Failed to open file in zip: %v", err)
			continue
		}
		processFile(File{
			Name: f.Name,
			RC:   rc,
		}, filepath.Join(output, f.Name))

	}
}

func createZipFromDirectory(input, output string) {
	// create a new zip file
	newZipFile, err := os.Create(output)
	if err != nil {
		log.Fatalf("Failed to create new zip file: %v", err)
	}
	defer newZipFile.Close()

	w := zip.NewWriter(newZipFile)
	defer w.Close()

	err = filepath.Walk(input, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() {
			// Get relative path
			rel, err := filepath.Rel(input, path)
			if err != nil {
				return err
			}

			data, err := ioutil.ReadFile(path)
			if err != nil {
				return err
			}
			if !strings.HasSuffix(info.Name(), ".json") {
				// pass original file
				fw, err := w.Create(rel)
				if err != nil {
					return err
				}
				_, err = fw.Write(data)
				return nil
			}

			isProfile := strings.HasPrefix(info.Name(), constant.ProfileFile)

			// assuming Snapshot is a protobuf message
			var snapshot proto.Message = &pb.ChangeSnapshot{}
			if isProfile {
				snapshot = &pb.Profile{}
			}

			err = jsonpb.UnmarshalString(string(data), snapshot)
			if err != nil {
				snapshot = &pb.SnapshotWithType{}
				if err = jsonpb.UnmarshalString(string(data), snapshot); err != nil {
					return err
				}
			}

			pbData, err := proto.Marshal(snapshot)
			if err != nil {
				return err
			}

			name := strings.TrimSuffix(rel, ".json") + ".pb"
			if isProfile {
				name = strings.TrimSuffix(name, ".pb")
			}
			fw, err := w.Create(name)
			if err != nil {
				return err
			}

			_, err = fw.Write(pbData)
			if err != nil {
				return err
			}
		}

		return nil
	})

	if err != nil {
		log.Fatalf("Failed to process directory: %v", err)
	}
}

package api

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

var (
	// Trace logs general information messages.
	Trace *log.Logger
	// Error logs error messages.
	Error *log.Logger
)

// splits stdout into an array of lines, removing empty lines
func splitStdOutLines(stdout string) []string {
	lines := strings.Split(stdout, "\n")
	filteredLines := make([]string, 0)
	for _, line := range lines {
		if len(line) > 0 {
			filteredLines = append(filteredLines, line)
		}
	}
	return filteredLines
}

// splits stdout into an array of tokens, replacing tabs with spaces
func splitStdOutTokens(line string) []string {
	return strings.Fields(strings.ReplaceAll(line, "\t", " "))
}

// executes a command and returns the stdout as string
func execCommand(command string) (string, error) {
	if runtime.GOOS == "windows" {
		return execCommandWin(command)
	}
	stdout, err := exec.Command("bash", "-c", command).Output()
	return string(stdout), err
}

func execCommandWin(command string) (string, error) {
	// Splitting the command into the executable and the arguments
	// For Windows, commands are executed through cmd /C
	cmd := exec.Command("cmd", "/C", command)
	stdout, err := cmd.Output()
	return string(stdout), err
}

// checks if a string is contained in an array of strings
func contains(s []string, e string) bool {
	for _, a := range s {
		if a == e {
			return true
		}
	}
	return false
}

// Windows: returns a list of open ports for all instances of anytypeHelper.exe found using cli utilities tasklist, netstat and findstr
func getOpenPortsWindows() (map[string][]string, error) {
	appName := "anytypeHelper.exe"
	stdout, err := execCommand(`tasklist`)
	if err != nil {
		return nil, err
	}

	lines := splitStdOutLines(stdout)
	pids := map[string]bool{}
	for _, line := range lines {
		if !strings.Contains(line, appName) {
			continue
		}
		tokens := splitStdOutTokens(line)
		pids[tokens[1]] = true
	}

	if len(pids) == 0 {
		return nil, errors.New("application not running")
	}

	result := map[string][]string{}
	for pid := range pids {
		stdout, err := execCommand(`netstat -ano`)
		if err != nil {
			return nil, err
		}

		lines := splitStdOutLines(stdout)
		ports := map[string]bool{}
		for _, line := range lines {
			if !strings.Contains(line, pid) || !strings.Contains(line, "LISTENING") {
				continue
			}
			tokens := splitStdOutTokens(line)
			port := strings.Split(tokens[1], ":")[1]
			ports[port] = true
		}

		portsSlice := []string{}
		for port := range ports {
			portsSlice = append(portsSlice, port)
		}

		result[pid] = portsSlice
	}

	return result, nil
}

func isFileGateway(port string) (bool, error) {
	client := &http.Client{}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", "http://127.0.0.1:"+port+"/file", nil)
	if err != nil {
		return false, err
	}
	// disable follow redirect
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}

	resp, err := client.Do(req)
	if err != nil {
		return false, err
	}

	bu := bytes.NewBuffer(nil)
	if err := resp.Request.Write(bu); err != nil {
		return false, err
	}
	if _, err := ioutil.ReadAll(bu); err != nil {
		return false, err
	}

	defer resp.Body.Close()
	// should return 301 redirect Location: /file/
	if resp.StatusCode == 301 {
		return true, err
	}
	return false, err
}

func isGrpcWebServer(port string) (bool, error) {
	client := &http.Client{}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	var data = strings.NewReader(`AAAAAAIQFA==`)
	req, err := http.NewRequestWithContext(ctx, "POST", "http://127.0.0.1:"+port+"/anytype.ClientCommands/AppGetVersion", data)
	if err != nil {
		return false, err

	}
	req.Header.Set("Content-Type", "application/grpc-web-text")
	req.Header.Set("X-Grpc-Web", "1")
	resp, err := client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	// should has Content-Type: application/grpc-web-text
	if resp.Header.Get("Content-Type") == "application/grpc-web-text" {
		return true, nil
	}

	return false, fmt.Errorf("unexpected content type: %s", resp.Header.Get("Content-Type"))
}

// MacOS and Linux: returns a list of all open ports for all instances of anytype found using cli utilities lsof and grep
func getOpenPortsUnix() (map[string][]string, error) {
	// execute the command
	appName := "anytype"
	// appName := "grpcserve"
	stdout, err := execCommand(`lsof -i -P -n | grep LISTEN | grep "` + appName + `"`)
	Trace.Print(`lsof -i -P -n | grep LISTEN | grep "` + appName + `"`)
	if err != nil {
		Trace.Print(err)
		return nil, err
	}
	// initialize the result map
	result := make(map[string][]string)
	// split the output into lines
	lines := splitStdOutLines(stdout)
	for _, line := range lines {

		// normalize whitespace and split into tokens
		tokens := splitStdOutTokens(line)
		pid := tokens[1]
		port := strings.Split(tokens[8], ":")[1]

		// add the port to the result map
		if _, ok := result[pid]; !ok {
			result[pid] = []string{}
		}

		if !contains(result[pid], port) {
			result[pid] = append(result[pid], port)
		}
	}

	if len(result) == 0 {
		return nil, errors.New("application not running")
	}

	return result, nil
}

// Windows, MacOS and Linux: returns a list of all open ports for all instances of anytype found using cli utilities
func getOpenPorts() (map[string][]string, error) {
	// Get Platform
	platform := runtime.GOOS
	var (
		ports map[string][]string
		err   error
	)
	//nolint:nestif
	if platform == "windows" {
		ports, err = getOpenPortsWindows()
		if err != nil {
			return nil, err
		}
	} else if platform == "darwin" {
		ports, err = getOpenPortsUnix()
		if err != nil {
			return nil, err
		}
	} else if platform == "linux" {
		ports, err = getOpenPortsUnix()
		if err != nil {
			return nil, err
		}
	} else {
		return nil, errors.New("unsupported platform")
	}
	totalPids := len(ports)
	for pid, pidports := range ports {
		var gatewayPort, grpcWebPort string
		var errs []error
		for _, port := range pidports {
			var (
				errDetectGateway, errDetectGrpcWeb error
				serviceDetected                    bool
			)
			if gatewayPort == "" {
				if serviceDetected, errDetectGateway = isFileGateway(port); serviceDetected {
					gatewayPort = port
				}
			}
			// in case we already detected grpcweb port skip this
			if !serviceDetected && grpcWebPort == "" {
				if serviceDetected, errDetectGrpcWeb = isGrpcWebServer(port); serviceDetected {
					grpcWebPort = port
				}
			}
			if !serviceDetected {
				// means port failed to detect either gateway or grpcweb
				errs = append(errs, fmt.Errorf("port: %s; gateway: %w; grpcweb: %w", port, errDetectGateway, errDetectGrpcWeb))
			}
		}
		if gatewayPort != "" && grpcWebPort != "" {
			ports[pid] = []string{grpcWebPort, gatewayPort}
		} else {
			Trace.Printf("can't detect ports. pid: %s; grpc: '%s'; gateway: '%s'; error: %v;", pid, grpcWebPort, gatewayPort, errs)
			delete(ports, pid)
		}
	}
	if len(ports) > 0 {
		Trace.Printf("found ports: %v", ports)
	} else {
		Trace.Printf("ports no able to detect for %d pids", totalPids)
	}
	return ports, nil
}

func getPorts() (map[string][]string, error) {
	Trace = log.New(os.Stdout, "TRACE: ", log.Ldate|log.Ltime|log.Lshortfile)
	Error = log.New(os.Stderr, "ERROR: ", log.Ldate|log.Ltime|log.Lshortfile)

	return getOpenPorts()
}

package internal

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"go.uber.org/atomic"
)

const NetworkLocal = "local"
const NetworkStaging = "staging"

type Event struct {
	MethodName        string `json:"method_name"`
	Duration          int64  `json:"duration"`
	Os                string `json:"os"`
	MiddlewareVersion string `json:"middleware_version"`
	Network           string `json:"network"`
}

func GetMiddlewareVersion() (string, error) {
	out, err := exec.Command("git", "describe", "--tags", "--always").Output()
	if err != nil {
		return "", err
	}
	middlewareVersion := strings.Trim(string(out), "\n")
	return middlewareVersion, nil
}

func GetOs() (string, error) {
	if runtime.GOOS == "windows" {
		return "windows", nil
	}
	if runtime.GOOS == "darwin" {
		return "macos", nil
	}
	return "", fmt.Errorf("unknown os " + runtime.GOOS)
}

func SendResultsToHttp(apiKey string, events []Event) error {
	payload := map[string]interface{}{
		"api_key": apiKey,
		"events":  events,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}

	req, err := http.NewRequest("POST", "https://telemetry.anytype.io/perfstand", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	fmt.Println("Results sent successfully!")
	return nil
}

func KillServer() error {
	var cmd string
	if runtime.GOOS == "windows" {
		cmd = `(Get-NetTCPConnection -LocalPort 31007 -State Listen | Select-Object -ExpandProperty OwningProcess | ForEach-Object { Stop-Process -Id $_ -Force -ErrorAction SilentlyContinue }) ; Write-Host "Server killed"`
	} else {
		cmd = "kill -9 $(lsof -i :31007 -t) ; echo \"Server killed\""
	}
	return ExecuteCommand(cmd)
}

func ExecuteCommand(command string) error {
	fmt.Println(command)
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("powershell", "-Command", command)
	} else {
		cmd = exec.Command("bash", "-c", command)
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		return err
	}
	return nil
}

func UnpackZip(path string, workspace string) error {
	if runtime.GOOS == "windows" {
		return ExecuteCommand("Expand-Archive -Path " + path + " -DestinationPath " + workspace + " -Force")
	}
	return ExecuteCommand("unzip -qq -o " + path + " -d " + workspace)
}

func BuildAnytype(err error) error {
	buildServer := exec.Command("make", "build-server")
	buildServer.Stdout = os.Stdout
	buildServer.Stderr = os.Stderr
	buildServer.Env = append(os.Environ(), "TAGS=noauth")

	err = buildServer.Run()
	return err
}

func LoadEnv(env string) (string, error) {
	res := os.Getenv(env)
	if res == "" {
		return "", fmt.Errorf("environment variable %s is not set", env)
	}
	return res, nil
}

func WinFixPath(winPath string) (string, error) {
	return strings.ReplaceAll(winPath, "\\", "/"), nil
}

func SetupWd() (string, error) {
	err := os.Chdir("../../..")
	if err != nil {
		return "", err
	}

	getwd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	if runtime.GOOS == "windows" {
		getwd, err = WinFixPath(getwd)
		if err != nil {
			return "", err
		}
	}

	fmt.Println("Current working directory:", getwd)
	return getwd, nil
}

func GrpcWorkspaceOpen(workspace string) string {
	if runtime.GOOS == "windows" {
		return `cmd.exe /c 'grpcurl -import-path ../anytype-heart/ -proto pb/protos/service/service.proto -plaintext -d "{\"spaceId\":\"` + workspace + `\"}" localhost:31007 anytype.ClientCommands.WorkspaceOpen'`
	}
	return `grpcurl -import-path ../anytype-heart/ -proto pb/protos/service/service.proto -plaintext -d '{
		   "spaceId": "` + workspace + `"
		}' localhost:31007 anytype.ClientCommands.WorkspaceOpen`
}

func GrpcWorkspaceCreate() string {
	if runtime.GOOS == "windows" {
		return `cmd.exe /c 'grpcurl -import-path ../anytype-heart/ -proto pb/protos/service/service.proto -plaintext -d "{}" localhost:31007 anytype.ClientCommands.WorkspaceCreate'`
	}
	return `grpcurl -import-path ../anytype-heart/ -proto pb/protos/service/service.proto -plaintext -d '{
		}' localhost:31007 anytype.ClientCommands.WorkspaceCreate`
}

func GrpcAccountSelect(accHash, workspace, networkMode, staging string) string {
	if runtime.GOOS == "windows" {
		staging, _ = WinFixPath(staging)
		return `cmd.exe /c 'grpcurl -import-path ../anytype-heart/ -proto pb/protos/service/service.proto -plaintext -d "{\"id\":\"` + accHash + `\",\"rootPath\":\"` + workspace + `\",\"disableLocalNetworkSync\":false,\"networkMode\":` + networkMode + `,\"networkCustomConfigFilePath\":\"` + staging + `\"}" localhost:31007 anytype.ClientCommands.AccountSelect'`
	}
	return `grpcurl -import-path ../anytype-heart/ -proto pb/protos/service/service.proto -plaintext -d '{
		   "id": "` + accHash + `",
		   "rootPath": "` + workspace + `",
		   "disableLocalNetworkSync": false,
		   "networkMode": ` + networkMode + `,
		   "networkCustomConfigFilePath": "` + staging + `"
		}' localhost:31007 anytype.ClientCommands.AccountSelect`
}

func GrpcWalletCreateSession(mnemonic string) string {
	if runtime.GOOS == "windows" {
		return `cmd.exe /c 'grpcurl -import-path ../anytype-heart/ -proto pb/protos/service/service.proto -plaintext -d "{\"mnemonic\":\"` + mnemonic + `\"}" localhost:31007 anytype.ClientCommands.WalletCreateSession'`
	}
	return `grpcurl -import-path ../anytype-heart/ -proto pb/protos/service/service.proto -plaintext -d '{
		   "mnemonic": "` + mnemonic + `"
		}' localhost:31007 anytype.ClientCommands.WalletCreateSession`
}

func GrpcWalletRecover(workspace, mnemonic string) string {
	if runtime.GOOS == "windows" {
		return `cmd.exe /c 'grpcurl -import-path ../anytype-heart/ -proto pb/protos/service/service.proto -plaintext -d "{\"rootPath\":\"` + workspace + `\",\"mnemonic\":\"` + mnemonic + `\"}" localhost:31007 anytype.ClientCommands.WalletRecover'`
	}
	return `grpcurl -import-path ../anytype-heart/ -proto pb/protos/service/service.proto -plaintext -d '{
		   "rootPath": "` + workspace + `",
		   "mnemonic": "` + mnemonic + `"
		}' localhost:31007 anytype.ClientCommands.WalletRecover`
}

func GrpcWalletCreate(workspace string) string {
	if runtime.GOOS == "windows" {
		return `cmd.exe /c 'grpcurl -import-path ../anytype-heart/ -proto pb/protos/service/service.proto -plaintext -d "{\"rootPath\":\"` + workspace + `\"}" localhost:31007 anytype.ClientCommands.WalletCreate'`
	}
	return `grpcurl -import-path ../anytype-heart/ -proto pb/protos/service/service.proto -plaintext -d '{
		   "rootPath": "` + workspace + `"
		}' localhost:31007 anytype.ClientCommands.WalletCreate`
}

func GrpcAccountCreate(workspace, networkMode, staging string) string {
	if runtime.GOOS == "windows" {
		staging, _ = WinFixPath(staging)
		return `cmd.exe /c 'grpcurl -import-path ../anytype-heart/ -proto pb/protos/service/service.proto -plaintext -d "{\"icon\":13,\"networkMode\":` + networkMode + `,\"storePath\":\"` + workspace + `\",\"networkCustomConfigFilePath\":\"` + staging + `\"}" localhost:31007 anytype.ClientCommands.AccountCreate'`
	}
	return `grpcurl -import-path ../anytype-heart/ -proto pb/protos/service/service.proto -plaintext -d '{
		   "icon": 13,
		   "networkMode": ` + networkMode + `,
		   "storePath": "` + workspace + `",
		   "networkCustomConfigFilePath": "` + staging + `"
		}' localhost:31007 anytype.ClientCommands.AccountCreate`
}

func GrpcInitialSetParameters() string {
	if runtime.GOOS == "windows" {
		return `cmd.exe /c 'grpcurl -import-path ../anytype-heart/ -proto pb/protos/service/service.proto -plaintext -d "{\"platform\":\"test\",\"version\":\"0.0.0-test\"}" localhost:31007 anytype.ClientCommands.InitialSetParameters'`
	}
	return `grpcurl -import-path ../anytype-heart/ -proto pb/protos/service/service.proto -plaintext -d '{
		   "platform": "test",
		   "version": "0.0.0-test"
		}' localhost:31007 anytype.ClientCommands.InitialSetParameters`
}

func StartAnytypeBackground() error {
	var runServer *exec.Cmd
	if runtime.GOOS == "windows" {
		runServer = exec.Command("./dist/server.exe")
	} else {
		runServer = exec.Command("./dist/server")
	}
	runServer.Stdout = os.Stdout
	runServer.Stderr = os.Stderr
	runServer.Env = append(os.Environ(), `ANYPROF=:6060`)
	err := runServer.Start()
	if err != nil {
		return err
	}

	// Wait for the server to start
	for {
		var cmd string
		if runtime.GOOS == "windows" {
			cmd = `$pids = (Get-NetTCPConnection -LocalPort 31007 -State Listen -ErrorAction SilentlyContinue | Select-Object -ExpandProperty OwningProcess); if ($pids) { Write-Output "Found process: $pids" } else { Write-Output "No process found"; exit 1 }`
		} else {
			cmd = `pids=$(lsof -i :31007 -t) && [ -n "$pids" ] && echo "Found process: $pids" || { echo "No process found"; exit 1; }`
		}

		err = ExecuteCommand(cmd)

		if err == nil {
			break
		} else {
			time.Sleep(10 * time.Second)
			fmt.Println("Waiting for the server to start...", err)
		}
	}
	return nil
}

func CollectGoroutines() ([]byte, error) {
	url := "http://localhost:6060/debug/pprof/goroutine?debug=1"

	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return body, nil
}

type Command struct {
	Command string
	Name    string
}

type MethodResult struct {
	MethodName      string
	NetworkMode     string
	Measurements    []int64
	CurrentMax      int64
	CurrentMaxIndex int64
	MaxTrace        []byte
}

func (mr *MethodResult) TryUpdateTrace(trace []byte) {
	mrLen := len(mr.Measurements) - 1
	if mr.CurrentMax < mr.Measurements[mrLen] {
		mr.CurrentMax = mr.Measurements[mrLen]
		mr.MaxTrace = trace
	}
}

func Convert(res map[string]*MethodResult) ([]Event, error) {
	middlewareVersion, err := GetMiddlewareVersion()
	if err != nil {
		return nil, err
	}
	osName, err := GetOs()
	if err != nil {
		return nil, err
	}

	var events []Event
	for _, value := range res {
		for _, duration := range value.Measurements {
			events = append(events, Event{
				MethodName:        value.MethodName,
				Duration:          duration,
				Os:                osName,
				MiddlewareVersion: middlewareVersion,
				Network:           value.NetworkMode,
			})
		}
	}
	return events, nil
}

type PerfResult = map[string]*MethodResult

func SaveMaxTracesToFiles(perfResult PerfResult) error {
	for key, result := range perfResult {
		if result.CurrentMax > 0 {
			fileName := fmt.Sprintf("goroutine_%s_%d_%d.log", result.MethodName, result.CurrentMax, result.CurrentMaxIndex)
			err := os.WriteFile(fileName, result.MaxTrace, 0644)
			if err != nil {
				return err
			}
			fmt.Printf("Saved MaxTrace for method %s to file: %s\n", key, fileName)
		}
	}
	return nil
}

func AssertFileExists(filePath string) error {
	_, err := os.Stat(filePath)
	if err != nil {
		return err
	}
	return nil
}

func TraceServer(currentOperation *atomic.String, done chan struct{}, wait chan map[string][]byte) {
	currentTraces := make(map[string][][]byte)
	for {
		select {
		case <-done:
			traces := make(map[string][]byte)
			for key, value := range currentTraces {
				if len(value) > 0 {
					traces[key] = value[len(value)/2]
				} else {
					traces[key] = nil
				}
			}
			wait <- traces
			fmt.Println("Goroutine stopped")
		default:
			time.Sleep(1 * time.Second)
			currentOperation := currentOperation.Load()
			if currentOperation != "" {
				bytes, err := CollectGoroutines()
				if err != nil {
					fmt.Println("Error collecting goroutines:", err)
				} else {
					if trace, ok := currentTraces[currentOperation]; ok {
						currentTraces[currentOperation] = append(trace, bytes)
					} else {
						currentTraces[currentOperation] = [][]byte{bytes}
					}
				}
			}
		}
	}
}

func Measure(grpcurlCommands []Command, currentOperation *atomic.String, result PerfResult) error {
	for _, cmd := range grpcurlCommands {
		if cmd.Name != "" {
			currentOperation.Store(cmd.Name)
		}
		start := time.Now().UnixMilli()
		err := ExecuteCommand(cmd.Command)
		if err != nil {
			return err
		}
		if val, ok := result[cmd.Name]; ok {
			val.Measurements = append(val.Measurements, time.Now().UnixMilli()-start)
		}
		currentOperation.Store("")
	}
	return nil
}

func StartWithTracing(currentOperation *atomic.String, done chan struct{}, wait chan map[string][]byte) error {
	go TraceServer(currentOperation, done, wait)
	err := KillServer()
	if err != nil {
		return err
	}

	err = StartAnytypeBackground()
	if err != nil {
		return err
	}
	return nil
}

func CollectMeasurements(
	grpcurlCommands []Command,
	currentOperation *atomic.String,
	result PerfResult,
	done chan struct{},
	wait chan map[string][]byte,
) error {
	err := Measure(grpcurlCommands, currentOperation, result)
	if err != nil {
		return err
	}

	err = KillServer()
	if err != nil {
		return err
	}

	fmt.Println("All commands executed successfully.")
	close(done)
	traces := <-wait
	for key, value := range traces {
		result[key].TryUpdateTrace(value)
	}
	return nil
}

func ReadJson[T any](t *T, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	err = json.Unmarshal(data, &t)
	if err != nil {
		return err
	}
	return nil
}

type BasicInput struct {
	NetworkMode string `json:"network_mode"`
	NodesConfig string `json:"nodes_config"`
	Times       int    `json:"times,omitempty"`
	Workspace   string `json:"workspace,omitempty"`
}

type BasicInputtable interface {
	ValidateNetwork() error
	SetTimes(times int)
	SetWorkspace(workspace string)
}

func (bi *BasicInput) ValidateNetwork() error {
	if bi.NetworkMode != NetworkLocal && bi.NetworkMode != NetworkStaging {
		return fmt.Errorf("network mode should be either 'local' or 'staging', got: %s", bi.NetworkMode)
	}
	if bi.NetworkMode == NetworkStaging {
		wd, err := os.Getwd()
		if err != nil {
			return err
		}
		bi.NodesConfig = filepath.Join(wd, bi.NodesConfig)
		err = AssertFileExists(bi.NodesConfig)
		if err != nil {
			return err
		}
	}
	return nil
}

func (bi *BasicInput) SetTimes(times int) {
	bi.Times = times
}

func (bi *BasicInput) SetWorkspace(workspace string) {
	bi.Workspace = workspace
}

func Prepare[T BasicInputtable](prep T, f func(T) error) error {
	configPath := os.Args[1]
	err := AssertFileExists(configPath)
	if err != nil {
		return err
	}

	times, err := strconv.Atoi(os.Args[2])
	if err != nil {
		return err
	}
	if times <= 0 {
		return fmt.Errorf("times should be greater than 0, got: %d", times)
	}
	prep.SetTimes(times)

	err = ReadJson(&prep, configPath)
	if err != nil {
		return err
	}

	workspace, err := os.MkdirTemp("", "workspace")
	if err != nil {
		return err
	}

	if runtime.GOOS == "windows" {
		winWorkspace, err := WinFixPath(workspace)
		if err != nil {
			return err
		}
		fmt.Println("Created temporary directory:", winWorkspace)
		prep.SetWorkspace(winWorkspace)
	} else {
		fmt.Println("Created temporary directory:", workspace)
		prep.SetWorkspace(workspace)
	}

	_, err = SetupWd()
	if err != nil {
		return err
	}

	err = prep.ValidateNetwork()
	if err != nil {
		return err
	}

	if f != nil {
		err = f(prep)
		if err != nil {
			return err
		}
	}

	if runtime.GOOS == "windows" {
		fmt.Printf("Build GRPC server on Windows...")
		command := `
			$Env:GOOS="windows";
			$Env:GOARCH="amd64";
			$Env:CGO_ENABLED="1";
			$Env:CC="x86_64-w64-mingw32-gcc";
			$Env:CXX="x86_64-w64-mingw32-g++";
			go build -o dist/server.exe -ldflags "$env:FLAGS -linkmode external -extldflags=-static" --tags "noauth nosigar nowatchdog" $env:BUILD_FLAGS github.com/anyproto/anytype-heart/cmd/grpcserver
		`
		err := ExecuteCommand(command)
		if err != nil {
			fmt.Printf("Error on building: %v\n", err)
		} else {
			fmt.Println("Build completed successfully")
		}
	} else {
		err = BuildAnytype(err)
		if err != nil {
			return err
		}
	}

	return nil
}

func SendResults(res PerfResult) error {
	apiKey, err := LoadEnv("CH_API_KEY")
	if err != nil {
		return err
	}

	events, err := Convert(res)
	if err != nil {
		return err
	}

	err = SendResultsToHttp(apiKey, events)
	if err != nil {
		return err
	}

	for key, value := range res {
		fmt.Printf("### Results::%s: %v\n", key, value.Measurements)
	}
	return nil
}

func After(res PerfResult) error {
	err := SendResults(res)
	if err != nil {
		return err
	}

	err = SaveMaxTracesToFiles(res)
	if err != nil {
		return err
	}
	return nil
}

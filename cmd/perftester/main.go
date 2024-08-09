package main

import (
	"fmt"
	"math"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

func main() {
	times, err := strconv.Atoi(os.Args[1])
	if err != nil {
		fmt.Println("Wrong number:", err)
		os.Exit(1)
	}

	err = os.Chdir("../..")
	if err != nil {
		fmt.Println("Error changing the directory:", err)
		os.Exit(1)
	}

	for i := 0; i < times; i++ {
		err := makeIteration()
		if err != nil {
			os.Exit(1)
		}
		time.Sleep(10 * time.Second)
	}
}

func makeIteration() error {
	err := executeCommand("kill $(lsof -i :31007 -t) ; echo \"Server killed\"")
	if err != nil {
		return err
	}

	getwd, err := os.Getwd()
	if err != nil {
		fmt.Println("Error getting the current working directory:", err)
		return err
	}
	fmt.Println("Current working directory:", getwd)

	err = os.MkdirAll("root", os.ModePerm)
	if err != nil {
		fmt.Println("Error creating directory:", err)
		return err
	}

	err = clearRoot()
	if err != nil {
		return err
	}

	testMnemonic := os.Getenv("TEST_MNEMONIC")

	if testMnemonic == "" {
		fmt.Println("Environment variable TEST_MNEMONIC is not set.")
		return err
	}

	reportMemory := os.Getenv("ANYTYPE_REPORT_MEMORY")
	if reportMemory == "" {
		fmt.Println("Environment variable ANYTYPE_REPORT_MEMORY is not set.")
		return err
	}

	// Execute the "make build-server" command
	buildServer := exec.Command("make", "build-server")
	buildServer.Stdout = os.Stdout
	buildServer.Stderr = os.Stderr
	err = buildServer.Run()
	if err != nil {
		fmt.Println("Error executing make build-server:", err)
		return err
	}

	// Start the server in the background
	runServer := exec.Command("make", "run-server")
	runServer.Stdout = os.Stdout
	runServer.Stderr = os.Stderr
	err = runServer.Start()
	if err != nil {
		fmt.Println("Error starting the server:", err)
		return err
	}

	// Wait for 50 seconds
	time.Sleep(50 * time.Second)

	// grpcurl commands
	testRootPath := fmt.Sprintf("%s/root", getwd)

	grpcurlCommands := []string{
		`grpcurl -import-path ../anytype-heart/ -proto pb/protos/service/service.proto -plaintext -d '{
		   "platform": "test",
		   "version": "0.0.0-test"
		}' localhost:31007 anytype.ClientCommands.MetricsSetParameters`,

		`grpcurl -import-path ../anytype-heart/ -proto pb/protos/service/service.proto -plaintext -d '{
		   "rootPath": "` + testRootPath + `",
		   "mnemonic": "` + testMnemonic + `"
		}' localhost:31007 anytype.ClientCommands.WalletRecover`,

		`grpcurl -import-path ../anytype-heart/ -proto pb/protos/service/service.proto -plaintext -d '{
		   "mnemonic": "` + testMnemonic + `"
		}' localhost:31007 anytype.ClientCommands.WalletCreateSession`,

		`grpcurl -import-path ../anytype-heart/ -proto pb/protos/service/service.proto -plaintext -d '{
		   "id": "A9REkpZLXiG8FEQPepwzZEmrRZqbhXacsJaPp831EjNkRbXf",
		   "rootPath": "` + testRootPath + `",
		   "disableLocalNetworkSync": false,
		   "networkMode": 0,
		   "networkCustomConfigFilePath": ""
		}' localhost:31007 anytype.ClientCommands.AccountSelect`,
	}

	for _, cmd := range grpcurlCommands {
		start := time.Now().UnixMilli()
		err := executeCommand(cmd)
		if err != nil {
			return err
		}
		if strings.Contains(cmd, "anytype.ClientCommands.AccountSelect") {
			err = os.WriteFile("root/ACCOUNT_SELECT_TIME", []byte(strconv.FormatInt(int64(math.Abs(float64(time.Now().UnixMilli()-start))), 10)), 0644)
			if err != nil {
				fmt.Println("ACCOUNT_SELECT_TIME err:", err)
				return err
			}
		}
	}

	_ = runServer.Wait()
	err = executeCommand("kill $(lsof -i :31007 -t) ; echo \"Server killed\"")
	if err != nil {
		return err
	}

	// Remove the contents of the "root" directory
	err = clearRoot()
	if err != nil {
		return err
	}

	fmt.Println("All commands executed successfully.")
	return nil
}

func clearRoot() error {
	err := os.RemoveAll("root")
	if err != nil {
		fmt.Println("Error removing the root directory:", err)
		return err
	}
	return nil
}

func executeCommand(command string) error {
	fmt.Println(command)
	cmd := exec.Command("bash", "-c", command)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		fmt.Println("Error executing command:", err)
		return err
	}
	return nil
}

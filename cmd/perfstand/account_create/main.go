package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"

	"go.uber.org/atomic"

	"github.com/anyproto/anytype-heart/cmd/perfstand/internal"
)

const AccountCreate = "AccountCreate"

type input struct {
	*internal.BasicInput
}

type wallet struct {
	Mnemonic string `json:"mnemonic"`
}

func NewInput() *input {
	res := new(input)
	res.BasicInput = new(internal.BasicInput)
	return res
}

func NewResults(networkMode string) internal.PerfResult {
	return internal.PerfResult{
		AccountCreate: {MethodName: AccountCreate, NetworkMode: networkMode},
	}
}


func main() {
	prep := NewInput()
	err := internal.Prepare(prep, nil)
	if err != nil {
		fmt.Println("Error preparing the environment:", err)
		os.Exit(1)
	}

	res := NewResults(prep.NetworkMode)
	for i := 0; i < prep.Times; i++ {
		err = iterate(prep, res)
		if err != nil {
			fmt.Println("Error making iteration:", err)
			os.Exit(1)
		}
	}
	err = internal.After(res)
	if err != nil {
		fmt.Println("Error after the test:", err)
		os.Exit(1)
	}
}

func iterate(prep *input, result internal.PerfResult) error {
	workspace, err := os.MkdirTemp("", "workspace")
	if err != nil {
		return err
	}

	if runtime.GOOS == "windows" {
		workspace, err = internal.WinFixPath(workspace)
		if err != nil {
			return err
		}
	}

	prep.Workspace = workspace
	defer os.RemoveAll(workspace)
	fmt.Println("Created temporary directory:", workspace)

	var currentOperation atomic.String
	done := make(chan struct{})
	wait := make(chan map[string][]byte)

	err = internal.StartWithTracing(&currentOperation, done, wait)
	if err != nil {
		return err
	}

	err = internal.ExecuteCommand(internal.GrpcInitialSetParameters())
	if err != nil {
		return err
	}

	var walletStr []byte
	if runtime.GOOS == "windows" {
		walletStr, err = exec.Command("powershell", "-Command", internal.GrpcWalletCreate(workspace)).Output()
	} else {
		walletStr, err = exec.Command("bash", "-c", internal.GrpcWalletCreate(workspace)).Output()
	}

	if err != nil {
		return err
	}

	var wallet wallet
	err = json.Unmarshal(walletStr, &wallet)
	if err != nil {
		return err
	}

	grpcurlCommands := []internal.Command{
		{internal.GrpcWalletCreateSession(wallet.Mnemonic), ""},
		accountCreate(prep),
	}

	err = internal.CollectMeasurements(grpcurlCommands, &currentOperation, result, done, wait)
	if err != nil {
		return err
	}
	return nil
}

func accountCreate(prep *input) internal.Command {
	if prep.NetworkMode != internal.NetworkLocal {
		return internal.Command{Command: internal.GrpcAccountCreate(prep.Workspace, "2", prep.NodesConfig), Name: AccountCreate}
	}
	return internal.Command{Command: internal.GrpcAccountCreate(prep.Workspace, "1", ""), Name: AccountCreate}
}

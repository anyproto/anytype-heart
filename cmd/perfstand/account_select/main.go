package main

import (
	"fmt"
	"os"
	"path/filepath"

	"go.uber.org/atomic"

	"github.com/anyproto/anytype-heart/cmd/perfstand/internal"
)

const AccountSelect = "AccountSelect"
const WorkspaceOpen = "WorkspaceOpen"
const WorkspaceCreate = "WorkspaceCreate"

type input struct {
	*internal.BasicInput
	RootPath string `json:"root_path"`
	AccHash  string `json:"acc_hash"`
	Mnemonic string `json:"mnemonic"`
	Space    string `json:"space"`
}

func NewInput() *input {
	res := new(input)
	res.BasicInput = new(internal.BasicInput)
	return res
}

func NewResults(networkMode string) internal.PerfResult {
	return internal.PerfResult{
		AccountSelect:   {MethodName: AccountSelect, NetworkMode: networkMode},
		WorkspaceOpen:   {MethodName: WorkspaceOpen, NetworkMode: networkMode},
		WorkspaceCreate: {MethodName: WorkspaceCreate, NetworkMode: networkMode},
	}
}

func main() {
	prep := NewInput()
	err := internal.Prepare(prep, extractAcc)
	if err != nil {
		fmt.Println("Error preparing the environment:", err)
		os.Exit(1)
	}
	defer os.RemoveAll(prep.Workspace)

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

func extractAcc(input *input) error {
	err := internal.UnpackZip(filepath.Join(input.RootPath, input.AccHash+".zip"), input.Workspace)
	if err != nil {
		return err
	}
	fmt.Println("Unpacked files to:", input.Workspace)
	return nil
}

func iterate(prep *input, result internal.PerfResult) error {
	var currentOperation atomic.String
	done := make(chan struct{})
	wait := make(chan map[string][]byte)

	err := internal.StartWithTracing(&currentOperation, done, wait)
	if err != nil {
		return err
	}

	grpcurlCommands := []internal.Command{
		{internal.GrpcInitialSetParameters(), ""},
		{internal.GrpcWalletRecover(prep.Workspace, prep.Mnemonic), ""},
		{internal.GrpcWalletCreateSession(prep.Mnemonic), ""},
		accountSelect(prep),
		{internal.GrpcWorkspaceOpen(prep.Space), WorkspaceOpen},
		{internal.GrpcWorkspaceCreate(), WorkspaceCreate},
	}

	err = internal.CollectMeasurements(grpcurlCommands, &currentOperation, result, done, wait)
	if err != nil {
		return err
	}
	return nil
}

func accountSelect(prep *input) internal.Command {
	if prep.NetworkMode != internal.NetworkLocal {
		return internal.Command{Command: internal.GrpcAccountSelect(prep.AccHash, prep.Workspace, "2", prep.NodesConfig), Name: AccountSelect}
	}
	return internal.Command{Command: internal.GrpcAccountSelect(prep.AccHash, prep.Workspace, "1", ""), Name: AccountSelect}
}

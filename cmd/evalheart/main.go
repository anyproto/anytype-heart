// Command evalheart boots a headless heart on a throwaway account for the
// MCP scenario campaign (scripts/mcp_eval/run_campaign.py): it builds this
// tree's cmd/grpcserver through heartboot, creates an empty local account,
// mints a JSON-API key, prints one JSON line — the API URL, the key, the
// account id, the data and log paths — and then waits. SIGTERM, SIGINT or
// stdin closing tears the heart down and deletes the account directory
// (unless -keep-account).
//
// The key goes to stdout, read by the parent through a pipe; it is never on
// a command line or in a file.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/anyproto/anytype-heart/cmd/apiv2eval/heartboot"
)

type ready struct {
	APIURL    string `json:"api_url"`
	APIKey    string `json:"api_key"`
	AccountId string `json:"account_id"`
	DataDir   string `json:"data_dir"`
	LogPath   string `json:"log_path"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "evalheart:", err)
		os.Exit(1)
	}
}

func run() error {
	binary := flag.String("heart-binary", "", "a prebuilt cmd/grpcserver (default: build this tree's)")
	keep := flag.Bool("keep-account", false, "keep the account directory after teardown")
	appName := flag.String("app-name", "mcp-eval", "the key's app name, stamped on every object the campaign creates")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	heart, err := heartboot.Start(ctx, heartboot.Options{
		BinaryPath: *binary, KeepDataDir: *keep, AccountName: *appName, AppName: *appName, Log: os.Stderr,
	})
	if err != nil {
		return fmt.Errorf("start heart: %w", err)
	}
	defer func() {
		if err := heart.Stop(); err != nil {
			fmt.Fprintln(os.Stderr, "evalheart: stop:", err)
		}
	}()
	line, err := json.Marshal(ready{APIURL: heart.APIURL, APIKey: heart.APIKey, AccountId: heart.AccountId,
		DataDir: heart.DataDir, LogPath: heart.LogPath})
	if err != nil {
		return fmt.Errorf("encode ready line: %w", err)
	}
	fmt.Println(string(line))

	stdinClosed := make(chan struct{})
	go func() {
		_, _ = io.Copy(io.Discard, os.Stdin)
		close(stdinClosed)
	}()
	select {
	case <-ctx.Done():
	case <-stdinClosed:
	}
	return nil
}

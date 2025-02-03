package shell

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

func NewShellCmd(rootCmd *cobra.Command) *cobra.Command {
	return &cobra.Command{
		Use:   "shell",
		Short: "Start the Anytype interactive shell",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("Starting Anytype interactive shell. Type 'exit' to quit.")
			return runShell(rootCmd)
		},
	}
}

func runShell(rootCmd *cobra.Command) error {
	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Print(">>> ")
		line, err := reader.ReadString('\n')
		if err != nil {
			fmt.Println("Error reading input:", err)
			continue
		}
		line = strings.TrimSpace(line)

		if line == "exit" || line == "quit" {
			fmt.Println("Goodbye!")
			return nil
		}

		if line == "" {
			continue // ignore empty input
		}

		args := strings.Split(line, " ")
		rootCmd.SetArgs(args)

		if err := rootCmd.Execute(); err != nil {
			fmt.Println("Command error:", err)
		}
	}
}

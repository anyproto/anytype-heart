package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var shellCmd = &cobra.Command{
	Use:   "shell",
	Short: "Start the Anytype interactive shell",
	Run: func(cmd *cobra.Command, args []string) {
		reader := bufio.NewReader(os.Stdin)
		for {
			fmt.Print(">>> ")
			line, err := reader.ReadString('\n')
			if err != nil {
				fmt.Println("Error reading line:", err)
				return
			}
			line = strings.TrimSpace(line)

			if line == "exit" || line == "quit" {
				fmt.Println("Bye!")
				return
			}

			parts := strings.Split(line, " ")

			if len(parts[0]) == 0 {
				continue
			}

			rootCmd.SetArgs(parts)
			if err := rootCmd.Execute(); err != nil {
				fmt.Println("Command error:", err)
			}
		}
	},
}

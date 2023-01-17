package console

import (
	"fmt"
	"os"

	"github.com/logrusorgru/aurora"
)

func Success(format string, args ...interface{}) {
	fmt.Println(aurora.Sprintf(aurora.Green("> Success! %s"),
		aurora.Sprintf(aurora.BrightBlack(format), args...)))
}

func Fatal(format string, args ...interface{}) {
	fmt.Println(aurora.Sprintf(aurora.Red("> Fatal! %s"),
		aurora.Sprintf(aurora.BrightBlack(format), args...)))
	os.Exit(1)
}

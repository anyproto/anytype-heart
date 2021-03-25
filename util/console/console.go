package console

import (
	"fmt"
	"os"

	"github.com/logrusorgru/aurora"
)

func Message(format string, args ...interface{}) {
	if len(format) > 0 {
		fmt.Println(aurora.Sprintf(aurora.BrightBlack("> "+format), args...))
	}
}

func Success(format string, args ...interface{}) {
	fmt.Println(aurora.Sprintf(aurora.Green("> Success! %s"),
		aurora.Sprintf(aurora.BrightBlack(format), args...)))
}

func Warn(format string, args ...interface{}) {
	fmt.Println(aurora.Sprintf(aurora.Magenta("> Warning! %s"),
		aurora.Sprintf(aurora.BrightBlack(format), args...)))
}

func Error(format string, args ...interface{}) {
	fmt.Println(aurora.Sprintf(aurora.Yellow("> Error! %s"),
		aurora.Sprintf(aurora.BrightBlack(format), args...)))
}

func End(format string, args ...interface{}) {
	Message(format, args...)
	os.Exit(0)
}

func Fatal(format string, args ...interface{}) {
	fmt.Println(aurora.Sprintf(aurora.Red("> Fatal! %s"),
		aurora.Sprintf(aurora.BrightBlack(format), args...)))
	os.Exit(1)
}

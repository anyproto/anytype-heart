package service

import (
	"os/exec"
	"strings"
	"time"
)

func fixTZ() {
	tzName, err := exec.Command("/system/bin/getprop", "persist.sys.timezone").Output()
	if err != nil {
		fmt.Printf("failed to get system timezone: %s\n", err.Error())
		return
	}
	tzName = strings.TrimSpace(string(tzName))
	z, err := time.LoadLocation(tzName)
	if err != nil {
		fmt.Printf("failed to load tz %s: %s\n", tzName, err.Error())
		return
	}
	time.Local = z
}

package service

import (
	"bytes"
	"fmt"
	"os/exec"
	"time"
)

func fixTZ() {
	tzName, err := exec.Command("/system/bin/getprop", "persist.sys.timezone").Output()
	if err != nil {
		fmt.Printf("failed to get system timezone: %s\n", err.Error())
		return
	}
	tzName = bytes.TrimSpace(tzName)
	z, err := time.LoadLocation(string(tzName))
	if err != nil {
		fmt.Printf("failed to load tz %s: %s\n", tzName, err.Error())
		return
	}
	time.Local = z
}

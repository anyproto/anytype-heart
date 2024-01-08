package service

import (
	"strings"
	"time"

	"ObjC/Foundation/NSTimeZone"
)

func fixTZ() {
	NSTimeZone.SystemTimeZone()
	z, _ := time.LoadLocation(strings.Split(NSTimeZone.SystemTimeZone().Description(), " ")[0])
	time.Local = z
}

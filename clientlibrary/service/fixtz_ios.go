//go:build cgo && ios

package service

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Foundation
#import <Foundation/Foundation.h>

const char* getSystemTimeZone() {
    NSTimeZone *timeZone = [NSTimeZone systemTimeZone];
    NSString *timeZoneName = [timeZone description];
    return [timeZoneName UTF8String];
}
*/
import "C"

import (
	"fmt"
	"strings"
	"time"
)

func getSystemTimeZone() string {
	tz := C.getSystemTimeZone()
	return C.GoString(tz)
}

func fixTZ() {
	tzDesc := getSystemTimeZone()
	if len(tzDesc) == 0 {
		fmt.Printf("failed to get system timezone\n")
		return
	}
	tzName := strings.Split(tzDesc, " ")[0]
	z, err := time.LoadLocation(tzName)
	if err != nil {
		fmt.Printf("failed to load tz %s: %s\n", tzName, err.Error())
		return
	}
	time.Local = z
}

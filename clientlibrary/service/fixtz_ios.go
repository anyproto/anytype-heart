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
	"strings"
	"time"
)

func getSystemTimeZone() string {
	tz := C.getSystemTimeZone()
	return C.GoString(tz)
}

func fixTZ() {
	z, _ := time.LoadLocation(strings.Split(getSystemTimeZone(), " ")[0])
	time.Local = z
}

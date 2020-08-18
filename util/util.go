package util

import (
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	ma "github.com/multiformats/go-multiaddr"
)

func MultiAddressesToStrings(addrs []ma.Multiaddr) []string {
	var s []string
	for _, addr := range addrs {
		s = append(s, addr.String())
	}

	return s
}

func TruncateText(text string, length int) string {
	var ellipsis = " …"
	if utf8.RuneCountInString(text) <= length {
		return text
	}

	var lastWordIndex, lastNonSpace, currentLen, endTextPos int
	for i, r := range text {
		currentLen++
		if unicode.IsSpace(r) {
			lastWordIndex = lastNonSpace
		} else if unicode.In(r, unicode.Han, unicode.Hangul, unicode.Hiragana, unicode.Katakana) {
			lastWordIndex = i
		} else {
			lastNonSpace = i + utf8.RuneLen(r)
		}

		if currentLen > length {
			if lastWordIndex == 0 {
				endTextPos = i
			} else {
				endTextPos = lastWordIndex
			}
			out := text[0:endTextPos]

			return out + ellipsis
		}
	}

	return text
}

func NewImmediateTicker(d time.Duration) *immediateTicker {
	c := make(chan time.Time, 1)
	s := make(chan struct{})

	ticker := time.NewTicker(d)
	c <- time.Now()

	go func() {
		for {
			select {
			case t := <-ticker.C:
				c <- t
			case <-s:
				ticker.Stop()
				return
			}
		}
	}()

	return &immediateTicker{C: c, s: s}
}

type immediateTicker struct {
	C    chan time.Time
	s    chan struct{}
	stop sync.Once
}

func (t *immediateTicker) Stop() {
	t.stop.Do(func() { close(t.s) })
}

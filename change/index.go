package change

import "github.com/anytypeio/go-anytype-middleware/change/log"

type logSource struct {
	breakpoint string
	*log.Log
}


type logFinder struct {

}

func (lf *logFinder) Find(logs []*log.Log) []logSource {
	var logByHead = make(map[string]*log.Log)
	for _, l := range logs {
		logByHead[l.Head] = l
	}

	for _, l := range logs {

	}
	return nil
}

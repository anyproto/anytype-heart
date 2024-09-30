//  Copyright (c) 2014 Couchbase, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// 		http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package json

import (
	"encoding/json"

	"github.com/blevesearch/bleve/v2/registry"
	"github.com/blevesearch/bleve/v2/search/highlight"

	"github.com/anyproto/anytype-heart/pkg/lib/logging"
)

var log = logging.Logger("ftsearch")

const Name = "json"

const separator = "…"

type FragmentFormatter struct {
}

type Fragment struct {
	Text   []byte   `json:"t"`
	Ranges [][2]int `json:"r"`
}

func NewFragmentFormatter() *FragmentFormatter {
	return &FragmentFormatter{}
}

func UnmarshalFromString(s string) (*Fragment, error) {
	var fragment Fragment
	err := json.Unmarshal([]byte(s), &fragment)
	return &fragment, err
}

func (a *FragmentFormatter) Format(f *highlight.Fragment, orderedTermLocations highlight.TermLocations) string {
	curr := f.Start

	fragment := Fragment{
		Text: f.Orig[f.Start:f.End],
	}
	if f.Start != 0 {
		// add additional offset for the separator
		// so all ranges will be shifted left by the length of the separator
		f.Start -= len(separator)
		fragment.Text = append([]byte(separator), fragment.Text...)
	}
	if f.End != len(f.Orig) {
		fragment.Text = append(fragment.Text, []byte(separator)...)
	}

	for _, termLocation := range orderedTermLocations {
		if termLocation == nil {
			continue
		}
		// make sure the array positions match
		if !termLocation.ArrayPositions.Equals(f.ArrayPositions) {
			continue
		}
		if termLocation.Start < curr {
			continue
		}
		if termLocation.End > f.End {
			break
		}

		fragment.Ranges = append(fragment.Ranges, [2]int{termLocation.Start - f.Start, termLocation.End - f.Start})
		// add the stuff before this location
		// start the <mark> tag
		curr = termLocation.End
	}

	b, err := json.Marshal(fragment)
	if err != nil {
		log.Warnf("error marshaling fragment: %v", err)
		return ""
	}
	return string(b)
}

func Constructor(config map[string]interface{}, cache *registry.Cache) (highlight.FragmentFormatter, error) {
	return NewFragmentFormatter(), nil
}

func init() {
	registry.RegisterFragmentFormatter(Name, Constructor)
}

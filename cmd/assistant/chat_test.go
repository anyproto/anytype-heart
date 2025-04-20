package main

import (
	"reflect"
	"testing"
)

func TestExtractObjectIDs(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantIDs  []string
		wantText string
	}{
		{
			name:     "with two IDs and no space",
			input:    "<<<object1,object2>>>Hello world",
			wantIDs:  []string{"object1", "object2"},
			wantText: "Hello world",
		},
		{
			name:     "with three IDs and leading space in rest",
			input:    "<<<id1,id2,id3>>>   This is a test",
			wantIDs:  []string{"id1", "id2", "id3"},
			wantText: "This is a test",
		},
		{
			name:     "empty IDs",
			input:    "<<<>>>No IDs here",
			wantIDs:  nil,
			wantText: "No IDs here",
		},
		{
			name:     "no header at all",
			input:    "Just a normal message",
			wantIDs:  nil,
			wantText: "Just a normal message",
		},
		{
			name:     "malformed header missing closing",
			input:    "<<<id1,id2 No closing delimiter",
			wantIDs:  nil,
			wantText: "<<<id1,id2 No closing delimiter",
		},
		{
			name:     "header only with trailing whitespace",
			input:    "<<<onlyone>>>",
			wantIDs:  []string{"onlyone"},
			wantText: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ids, text := ExtractObjectIDs(tt.input)
			if !reflect.DeepEqual(ids, tt.wantIDs) {
				t.Errorf("ExtractObjectIDs(%q) IDs = %v, want %v", tt.input, ids, tt.wantIDs)
			}
			if text != tt.wantText {
				t.Errorf("ExtractObjectIDs(%q) text = %q, want %q", tt.input, text, tt.wantText)
			}
		})
	}
}

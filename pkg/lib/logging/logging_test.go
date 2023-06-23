package logging

import (
	"reflect"
	"testing"

	"github.com/anyproto/any-sync/app/logger"
	"github.com/stretchr/testify/assert"
)

func TestLevelsFromStr(t *testing.T) {
	tests := []struct {
		name string
		arg  string
		want []logger.NamedLevel
	}{
		{
			name: "Correct Input",
			arg:  "name1=DEBUG;prefix*=WARN;*=ERROR",
			want: []logger.NamedLevel{
				{Name: "name1", Level: "DEBUG"},
				{Name: "prefix*", Level: "WARN"},
				{Name: "*", Level: "ERROR"},
			},
		},
		{
			name: "Correct Input with whitespaces",
			arg:  "name1 = DEBUG ; prefix* = WARN; *= ERROR",
			want: []logger.NamedLevel{
				{Name: "name1", Level: "DEBUG"},
				{Name: "prefix*", Level: "WARN"},
				{Name: "*", Level: "ERROR"},
			},
		},
		{
			name: "Extra semicolon",
			arg:  "name1=DEBUG;prefix*=WARN;*=ERROR;",
			want: []logger.NamedLevel{
				{Name: "name1", Level: "DEBUG"},
				{Name: "prefix*", Level: "WARN"},
				{Name: "*", Level: "ERROR"},
			},
		},
		{
			name: "Invalid level",
			arg:  "name1=DEBUG;prefix*=WARN;*=INVALID",
			want: []logger.NamedLevel{
				{Name: "name1", Level: "DEBUG"},
				{Name: "prefix*", Level: "WARN"},
			},
		},
		{
			name: "Empty",
			arg:  "",
			want: nil,
		},
		{
			name: "spaces",
			arg:  "     ",
			want: nil,
		},
		{
			name: "invalid assignment",
			arg:  "a=b=c=d",
			want: nil,
		},
		{
			name: "wtf",
			arg:  "   ;fsg;;gf;gf;gd;gd;g;fd;dfg;;gfd----gd-gfd-g-gdf-gd-g-gd-fg-====gdf=gf==;;;==;=;=;=;=g  ",
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := LevelsFromStr(tt.arg)
			assert.True(t, reflect.DeepEqual(got, tt.want), "LevelsFromStr() = %v, want %v", got, tt.want)
		})
	}
}

package logging

import (
	"net/url"
	"reflect"
	"testing"

	"github.com/anyproto/any-sync/app/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSinkFilename(t *testing.T) {
	// zap resolves sink output paths via url.Parse. Windows paths start with a
	// drive letter, so there is no "/" after the scheme and the URL parses as
	// opaque, leaving u.Path empty. Regression guard: every platform's path
	// must resolve to a non-empty filename, otherwise lumberjack silently
	// falls back to os.TempDir() and no logs reach the expected directory.
	tests := []struct {
		name string
		path string
		want string
	}{
		{
			name: "unix absolute path",
			path: `/Users/user/Library/Application Support/anytype/common/logs/anytype.log`,
			want: `/Users/user/Library/Application Support/anytype/common/logs/anytype.log`,
		},
		{
			name: "windows path with backslashes",
			path: `C:\Users\user\AppData\Roaming\anytype\common\logs\anytype.log`,
			want: `C:\Users\user\AppData\Roaming\anytype\common\logs\anytype.log`,
		},
		{
			name: "windows path with forward slashes",
			path: `C:/Users/user/AppData/Roaming/anytype/common/logs/anytype.log`,
			want: `C:/Users/user/AppData/Roaming/anytype/common/logs/anytype.log`,
		},
		{
			name: "windows UNC path",
			path: `\\server\share\anytype\logs\anytype.log`,
			want: `\\server\share\anytype\logs\anytype.log`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// given: the output path zap is handed by registerLumberjackSink
			u, err := url.Parse(lumberjackScheme + ":" + tt.path)
			require.NoError(t, err)

			// when
			got := sinkFilename(u)

			// then
			assert.Equal(t, tt.want, got)
			assert.NotEmpty(t, got, "empty filename makes lumberjack fall back to os.TempDir()")
		})
	}
}

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

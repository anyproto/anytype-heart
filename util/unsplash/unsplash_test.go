package unsplash

import (
	"github.com/rwcarlsen/goexif/exif"
	"github.com/stretchr/testify/require"
	"io"
	"os"
	"testing"
)

func Test_injectArtistIntoExif(t *testing.T) {
	type args struct {
		filePath   string
		artistName string
		artistUrl  string
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "1",
			args: args{
				filePath:   "../../core/block/testdata/testdir/a.jpg",
				artistName: "anytype",
				artistUrl:  "https://anytype.io",
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			copiedFile, err := os.CreateTemp(os.TempDir(), "anytype")
			require.NoError(t, err)
			defer copiedFile.Close()
			origFile, err := os.Open(tt.args.filePath)
			require.NoError(t, err)

			_, err = io.Copy(copiedFile, origFile)
			require.NoError(t, err)

			if err := injectArtistIntoExif(copiedFile.Name(), tt.args.artistName, tt.args.artistUrl); (err != nil) != tt.wantErr {
				t.Errorf("injectExif() error = %v, wantErr %v", err, tt.wantErr)
			}
			r, err := os.Open(copiedFile.Name())
			require.NoError(t, err)

			exf, err := exif.Decode(r)
			require.NoError(t, err)

			f, err := exf.Get(exif.Artist)
			require.NoError(t, err)
			s, err := f.StringVal()
			require.NoError(t, err)

			require.Equal(t, PackArtistNameAndURL(tt.args.artistName, tt.args.artistUrl), s)
		})
	}
}

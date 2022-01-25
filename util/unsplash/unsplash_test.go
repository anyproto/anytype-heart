package unsplash

import (
	"context"
	"github.com/hbagdi/go-unsplash/unsplash"
	"github.com/rwcarlsen/goexif/exif"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
	"io"
	"os"
	"testing"
	"time"
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

func Test_token(t *testing.T) {

	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: UNSPLASH_TOKEN},
	)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()
	client := oauth2.NewClient(ctx, ts)

	unsplashApi := unsplash.New(client)
	u, _, err := unsplashApi.CurrentUser()
	require.NoError(t, err)
	require.NotNil(t, u)
	require.Equal(t, "anytype", *u.Username)
}

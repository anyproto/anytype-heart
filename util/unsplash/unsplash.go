package unsplash

import (
	"context"
	"fmt"
	"github.com/dsoprea/go-exif/v3"
	jpegstructure "github.com/dsoprea/go-jpeg-image-structure/v2"
	"github.com/hbagdi/go-unsplash/unsplash"
	"golang.org/x/oauth2"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const UNSPLASH_TOKEN = "wZ8VMd2YU6JIzur4Whjsbe2IjDVHkE7uJ_xQRQbXkEc"

// todo: should probably add some GC here
var queryCache = newCacheWithTTL(time.Minute * 60)

// exitArtistWithUrl matches and extracts additional information we store in the Artist field – the URL of the author page.
// We use it within the Unsplash integration
var exitArtistWithUrl = regexp.MustCompile(`(.*?); (http.*?)`)

type Result struct {
	ID              string
	PictureThumbUrl string
	PictureSmallUrl string
	PictureFullUrl  string
	Artist          string
	ArtistURL       string
}

func newFromPhoto(v unsplash.Photo) (Result, error) {
	if v.ID == nil || v.Urls == nil {
		return Result{}, fmt.Errorf("nil input from unsplash")
	}
	res := Result{ID: *v.ID}
	if v.Urls.Thumb != nil {
		res.PictureThumbUrl = v.Urls.Thumb.String()
	}
	if v.Urls.Small != nil {
		res.PictureThumbUrl = v.Urls.Small.String()
	}
	if v.Urls.Full != nil {
		res.PictureThumbUrl = v.Urls.Full.String()
	}
	if v.Photographer == nil {
		return res, nil
	}
	if v.Photographer.Name != nil {
		res.Artist = *v.Photographer.Name
	}
	if v.Photographer.Links != nil && v.Photographer.Links.HTML != nil {
		res.ArtistURL = v.Photographer.Links.HTML.String()
	}
	return res, nil
}

func Search(ctx context.Context, query string, max int) ([]Result, error) {
	query = strings.ToLower(strings.TrimSpace(query))

	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: UNSPLASH_TOKEN},
	)
	client := oauth2.NewClient(ctx, ts)
	var opt unsplash.RandomPhotoOpt
	unsplashApi := unsplash.New(client)
	cachedResults := queryCache.get(query)
	if cachedResults != nil {
		return cachedResults, nil
	}

	opt.Count = max
	opt.SearchQuery = query

	results, _, err := unsplashApi.Photos.Random(&opt)
	if err != nil || results == nil {
		return nil, err
	}

	var photos = make([]Result, 0, len(*results))
	for _, v := range *results {
		res, err := newFromPhoto(v)
		if err != nil {
			continue
		}

		photos = append(photos, res)
	}
	queryCache.set(query, photos)

	return photos, err
}

func Download(ctx context.Context, id string) (imgPath string, err error) {
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: UNSPLASH_TOKEN},
	)
	var picture Result
	for _, res := range queryCache.getLast() {
		if res.ID == id {
			picture = res
			break
		}
	}
	if picture.ID == "" {
		client := oauth2.NewClient(ctx, ts)
		unsplashApi := unsplash.New(client)
		res, _, err := unsplashApi.Photos.Photo(id, nil)
		if err != nil {
			return "", err
		}
		picture, err = newFromPhoto(*res)
		if err != nil {
			return "", err
		}
	}

	responseDownload, err := http.Get(picture.PictureFullUrl)
	if err != nil {
		return "", fmt.Errorf("failed to download file from unsplash: %s", err.Error())
	}
	defer responseDownload.Body.Close()
	tmpfile, err := ioutil.TempFile(os.TempDir(), "anytype-unsplash")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %s", err.Error())
	}
	_, _ = io.Copy(tmpfile, responseDownload.Body)
	tmpfile.Close()

	err = injectArtistIntoExif(tmpfile.Name(), picture.Artist, picture.ArtistURL)
	if err != nil {
		return "", fmt.Errorf("failed to inject exif: %s", err.Error())
	}
	p, err := filepath.Abs(tmpfile.Name())
	if err != nil {
		return "", fmt.Errorf("failed to inject exif: %s", err.Error())
	}
	return p, nil
}

func PackArtistNameAndURL(name, url string) string {
	return fmt.Sprintf("%s; %s", name, url)
}

func UnpackArtist(packed string) (name, url string) {
	artistParts := exitArtistWithUrl.FindStringSubmatch(packed)
	if len(artistParts) == 3 {
		return artistParts[1], artistParts[2]
	}

	return packed, ""
}

func injectArtistIntoExif(filePath, artistName, artistUrl string) error {
	jmp := jpegstructure.NewJpegMediaParser()
	intfc, err := jmp.ParseFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to open file to read exif: %s", err.Error())
	}
	sl := intfc.(*jpegstructure.SegmentList)
	rootIb, err := sl.ConstructExifBuilder()
	if err != nil {
		return err
	}
	ifdPath := "IFD0"
	ifdIb, err := exif.GetOrCreateIbFromRootIb(rootIb, ifdPath)
	// Artist key in decimal is 315 https://www.exiv2.org/tags.html
	err = ifdIb.SetStandard(315, PackArtistNameAndURL(artistName, artistUrl))
	err = sl.SetExif(rootIb)
	f, err := os.OpenFile(filePath, os.O_RDWR|os.O_TRUNC, 0755)
	defer f.Close()
	if err != nil {
		return fmt.Errorf("failed to open file to write exif: %s", err.Error())
	}
	err = sl.Write(f)
	if err != nil {
		return fmt.Errorf("failed to write exif: %s", err.Error())
	}
	return nil
}

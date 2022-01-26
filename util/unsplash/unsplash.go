package unsplash

import (
	"context"
	"fmt"
	"github.com/anytypeio/go-anytype-middleware/app"
	"github.com/anytypeio/go-anytype-middleware/core/configfetcher"
	"github.com/anytypeio/go-anytype-middleware/util/ocache"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
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

const (
	CName         = "unsplash"
	DEFAULT_TOKEN = "TLKq5P192MptAcTHnGM8WQPZV8kKNn1eT9FEi5Srem0"
	cacheTTL      = time.Minute * 10
	cacheGCPeriod = time.Minute * 5
)

type Unsplash interface {
	Search(ctx context.Context, query string, max int) ([]Result, error)
	Download(ctx context.Context, id string) (imgPath string, err error)

	app.Component
}

type unsplashService struct {
	cache  ocache.OCache
	client *unsplash.Unsplash
	limit  int
	config configfetcher.ConfigFetcher
}

func (l *unsplashService) Init(app *app.App) (err error) {
	l.cache = ocache.New(l.search, ocache.WithTTL(cacheTTL), ocache.WithGCPeriod(cacheGCPeriod))
	l.config = app.MustComponent(configfetcher.CName).(configfetcher.ConfigFetcher)
	return
}

func (l *unsplashService) Name() (name string) {
	return CName
}

func New() Unsplash {
	return &unsplashService{}
}

// exifArtistWithUrl matches and extracts additional information we store in the Artist field – the URL of the author page.
// We use it within the Unsplash integration
var exifArtistWithUrl = regexp.MustCompile(`(.*?); (http.*)`)

type Result struct {
	ID              string
	Description     string
	PictureThumbUrl string
	PictureSmallUrl string
	PictureFullUrl  string
	Artist          string
	ArtistURL       string
}

type results struct {
	results []Result
}

func (results) Close() error {
	return nil
}

func newFromPhoto(v unsplash.Photo) (Result, error) {
	if v.ID == nil || v.Urls == nil {
		return Result{}, fmt.Errorf("nil input from unsplash")
	}
	res := Result{ID: *v.ID}
	if v.Urls.Thumb != nil {
		res.PictureThumbUrl = v.Urls.Thumb.String()
	}
	if v.Description != nil && *v.Description != "" {
		res.Description = *v.Description
	} else if v.AltDescription != nil {
		res.Description = *v.AltDescription
	}
	if v.Urls.Small != nil {
		res.PictureSmallUrl = v.Urls.Small.String()
	}
	if v.Urls.Full != nil {
		res.PictureFullUrl = v.Urls.Full.String()
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

func (l *unsplashService) lazyInitClient() {
	if l.client != nil {
		return
	}
	cfg := l.config.GetCafeConfig()
	token := DEFAULT_TOKEN
	if configToken := pbtypes.GetString(cfg.Extra, "unsplash"); configToken != "" {
		token = configToken
	}
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)
	l.client = unsplash.New(oauth2.NewClient(context.Background(), ts))
}

func (l *unsplashService) Search(ctx context.Context, query string, limit int) ([]Result, error) {
	query = strings.ToLower(strings.TrimSpace(query))
	l.limit = limit
	v, err := l.cache.Get(ctx, query)
	if err != nil {
		return nil, err
	}

	if r, ok := v.(results); ok {
		return r.results, nil
	} else {
		panic("invalid cache value")
	}
}

func (l *unsplashService) search(ctx context.Context, query string) (ocache.Object, error) {
	l.lazyInitClient()
	query = strings.ToLower(strings.TrimSpace(query))

	var opt unsplash.RandomPhotoOpt

	opt.Count = l.limit
	opt.SearchQuery = query

	res, _, err := l.client.Photos.Random(&opt)
	if err != nil {
		if strings.Contains("404", err.Error()) {
			return nil, nil
		}
		return nil, err
	}

	if res == nil {
		return nil, nil
	}

	var photos = make([]Result, 0, len(*res))
	for _, v := range *res {
		res, err := newFromPhoto(v)
		if err != nil {
			continue
		}

		photos = append(photos, res)
	}

	return results{results: photos}, nil
}

func (l *unsplashService) Download(ctx context.Context, id string) (imgPath string, err error) {
	l.lazyInitClient()
	var picture Result
	l.cache.ForEach(func(v ocache.Object) (isContinue bool) {
		// todo: it will be better to save the last result, but we need another lock for this
		if r, ok := v.(results); ok {
			for _, res := range r.results {
				if res.ID == id {
					picture = res
					break
				}
			}
		}
		return picture.ID == ""
	})

	if picture.ID == "" {
		res, _, err := l.client.Photos.Photo(id, nil)
		if err != nil {
			return "", err
		}
		picture, err = newFromPhoto(*res)
		if err != nil {
			return "", err
		}
	}
	req, err := http.NewRequest("GET", picture.PictureFullUrl, nil)
	if err != nil {
		return "", err
	}
	req = req.WithContext(ctx)
	client := http.DefaultClient
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to download file from unsplash: %s", err.Error())
	}
	defer resp.Body.Close()
	tmpfile, err := ioutil.TempFile(os.TempDir(), picture.ID)
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %s", err.Error())
	}
	_, _ = io.Copy(tmpfile, resp.Body)
	tmpfile.Close()

	err = injectIntoExif(tmpfile.Name(), picture.Artist, picture.ArtistURL, picture.Description)
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
	artistParts := exifArtistWithUrl.FindStringSubmatch(packed)
	if len(artistParts) == 3 {
		return artistParts[1], artistParts[2]
	}

	return packed, ""
}

func injectIntoExif(filePath, artistName, artistUrl, description string) error {
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
	if err != nil {
		return err
	}
	// Artist key in decimal is 315 https://www.exiv2.org/tags.html
	err = ifdIb.SetStandard(315, PackArtistNameAndURL(artistName, artistUrl))
	err = ifdIb.SetStandard(270, description)
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

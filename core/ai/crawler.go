package ai

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/go-shiori/go-readability"
)

// FetchAndExtract fetches the given URL and extracts the main content.
func FetchAndExtract(url string) (string, error) {
	client := &http.Client{Timeout: 10 * time.Second}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; AnytypeBot/1.0; +https://anytype.io/bot)")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", errors.New("failed to fetch the URL")
	}

	htmlBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	article, err := readability.FromReader(bytes.NewReader(htmlBytes), nil)
	if err != nil {
		return "", err
	}
	return article.TextContent, nil
}

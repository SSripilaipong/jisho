package source

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	releaseAPI = "https://api.github.com/repos/scriptin/jmdict-simplified/releases/latest"
	userAgent  = "jisho-cli/1.0 (github.com/shsnail/jisho)"
)

// GithubFetcher fetches releases from the jmdict-simplified GitHub repository.
type GithubFetcher struct {
	client *http.Client
}

func NewGithubFetcher() *GithubFetcher {
	return &GithubFetcher{
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (f *GithubFetcher) LatestRelease() (*Release, error) {
	req, err := http.NewRequest("GET", releaseAPI, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch release: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}

	var gh struct {
		TagName string `json:"tag_name"`
		Assets  []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
			Size               int64  `json:"size"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&gh); err != nil {
		return nil, fmt.Errorf("decode release: %w", err)
	}

	rel := &Release{Version: gh.TagName}
	for _, a := range gh.Assets {
		rel.Assets = append(rel.Assets, Asset{
			Name:        a.Name,
			DownloadURL: a.BrowserDownloadURL,
			Size:        a.Size,
		})
	}
	return rel, nil
}

func (f *GithubFetcher) Download(url string, dest io.Writer, progress func(int64, int64)) error {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)

	// Use a longer timeout for large file downloads.
	client := &http.Client{Timeout: 30 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("download %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download returned %d for %s", resp.StatusCode, url)
	}

	size := resp.ContentLength
	var written int64
	buf := make([]byte, 32*1024)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := dest.Write(buf[:n]); werr != nil {
				return fmt.Errorf("write: %w", werr)
			}
			written += int64(n)
			if progress != nil {
				progress(written, size)
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read body: %w", err)
		}
	}
	return nil
}

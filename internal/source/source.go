package source

import "io"

// Asset represents a downloadable data file.
type Asset struct {
	Name        string
	DownloadURL string
	Size        int64
}

// Release holds metadata about a jmdict-simplified release.
type Release struct {
	Version string
	Assets  []Asset
}

// Fetcher can fetch release metadata and download assets.
type Fetcher interface {
	LatestRelease() (*Release, error)
	Download(url string, dest io.Writer, progress func(int64, int64)) error
}

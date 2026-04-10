package cmd

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/schollz/progressbar/v3"
	"github.com/spf13/cobra"

	jishodb "github.com/shsnail/jisho/internal/db"
	"github.com/shsnail/jisho/internal/importer"
	"github.com/shsnail/jisho/internal/source"
)

var forceUpdate bool

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Download and import the latest jmdict-simplified data",
	RunE:  runUpdate,
}

func init() {
	rootCmd.AddCommand(updateCmd)
	updateCmd.Flags().BoolVar(&forceUpdate, "force", false, "re-import even if already up to date")
}

// assetSpec describes a data file we want from the release.
type assetSpec struct {
	// prefix that the asset name must contain
	prefix    string
	suffix    string
	importerF func() importer.Importer
	label     string
}

var wantedAssets = []assetSpec{
	{prefix: "jmdict-eng-", suffix: ".json.zip", importerF: func() importer.Importer { return importer.JMdictImporter{} }, label: "JMdict (words)"},
	{prefix: "jmnedict-all-", suffix: ".json.zip", importerF: func() importer.Importer { return importer.JMnedictImporter{} }, label: "JMnedict (names)"},
	{prefix: "kanjidic2-en-", suffix: ".json.zip", importerF: func() importer.Importer { return importer.KanjidicImporter{} }, label: "Kanjidic2"},
}

func runUpdate(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	fetcher := source.NewGithubFetcher()

	fmt.Println("Checking latest release…")
	rel, err := fetcher.LatestRelease()
	if err != nil {
		return fmt.Errorf("fetch release info: %w", err)
	}
	fmt.Printf("Latest version: %s\n", rel.Version)

	// Check current version in DB (if it exists).
	if !forceUpdate {
		if current, err := currentVersion(resolveDBPath()); err == nil && current == rel.Version {
			fmt.Println("Already up to date.")
			return nil
		}
	}

	tmpPath := resolveDBPath() + ".tmp"
	// Clean up any previous failed import.
	os.Remove(tmpPath)

	tmpDB, err := jishodb.Open(tmpPath)
	if err != nil {
		return fmt.Errorf("create temp db: %w", err)
	}
	defer func() {
		tmpDB.Close()
		os.Remove(tmpPath)
	}()

	if err := jishodb.SetImportPragmas(tmpDB); err != nil {
		return fmt.Errorf("set import pragmas: %w", err)
	}

	// Download and import each asset.
	for _, spec := range wantedAssets {
		asset := findAsset(rel.Assets, spec.prefix, spec.suffix)
		if asset == nil {
			fmt.Printf("  WARNING: asset matching %s*%s not found in release, skipping.\n", spec.prefix, spec.suffix)
			continue
		}

		fmt.Printf("\nDownloading %s (%s)…\n", spec.label, asset.Name)
		data, err := downloadToMemory(ctx, fetcher, asset)
		if err != nil {
			return fmt.Errorf("download %s: %w", spec.label, err)
		}

		fmt.Printf("Importing %s…\n", spec.label)
		jsonReader, jsonSize, err := extractZipJSON(data, asset.Name)
		if err != nil {
			return fmt.Errorf("extract %s: %w", spec.label, err)
		}

		bar := progressbar.DefaultBytes(jsonSize, "  importing")
		imp := spec.importerF()
		if err := imp.Import(ctx, tmpDB, jsonReader, jsonSize, func(read, total int64) {
			bar.Set64(read)
		}); err != nil {
			bar.Finish()
			return fmt.Errorf("import %s: %w", spec.label, err)
		}
		bar.Finish()
	}

	if err := jishodb.RestoreDefaultPragmas(tmpDB); err != nil {
		return fmt.Errorf("restore pragmas: %w", err)
	}

	// Sanity check.
	var count int
	if err := tmpDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM words`).Scan(&count); err != nil || count < 100000 {
		return fmt.Errorf("sanity check failed: words count = %d (expected ≥ 100000)", count)
	}
	fmt.Printf("\nImported %d words.\n", count)

	tmpDB.Close()

	// Atomic rename: commit point.
	finalPath := resolveDBPath()
	if err := os.MkdirAll(filepath.Dir(finalPath), 0o755); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}
	if err := os.Rename(tmpPath, finalPath); err != nil {
		return fmt.Errorf("rename db: %w", err)
	}

	fmt.Printf("Database updated to %s at %s\n", rel.Version, finalPath)
	return nil
}

// downloadToMemory downloads an asset into a []byte buffer with a progress bar.
func downloadToMemory(_ context.Context, fetcher source.Fetcher, asset *source.Asset) ([]byte, error) {
	bar := progressbar.DefaultBytes(asset.Size, "  downloading")
	var buf bytes.Buffer
	err := fetcher.Download(asset.DownloadURL, io.MultiWriter(&buf, bar), func(read, total int64) {})
	bar.Finish()
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// extractZipJSON opens a .zip in memory and returns a reader for the first .json file inside.
func extractZipJSON(data []byte, zipName string) (io.Reader, int64, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, 0, fmt.Errorf("open zip %s: %w", zipName, err)
	}
	for _, f := range zr.File {
		if strings.HasSuffix(f.Name, ".json") {
			rc, err := f.Open()
			if err != nil {
				return nil, 0, fmt.Errorf("open zip entry %s: %w", f.Name, err)
			}
			// Read fully into buffer so we can report size.
			jsonBytes, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				return nil, 0, fmt.Errorf("read zip entry: %w", err)
			}
			return bytes.NewReader(jsonBytes), int64(len(jsonBytes)), nil
		}
	}
	return nil, 0, fmt.Errorf("no .json file found in %s", zipName)
}

// findAsset returns the first asset whose name contains prefix and suffix.
func findAsset(assets []source.Asset, prefix, suffix string) *source.Asset {
	for i := range assets {
		name := assets[i].Name
		if strings.HasPrefix(name, prefix) && strings.HasSuffix(name, suffix) {
			return &assets[i]
		}
	}
	return nil
}

// currentVersion reads jmdict_version from the existing database, if any.
func currentVersion(path string) (string, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return "", err
	}
	d, err := sql.Open("sqlite", path)
	if err != nil {
		return "", err
	}
	defer d.Close()
	var v string
	err = d.QueryRow(`SELECT value FROM source_meta WHERE key='jmdict_version'`).Scan(&v)
	return v, err
}

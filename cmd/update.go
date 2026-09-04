package cmd

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"database/sql"
	"errors"
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

// jmdictXMLURL is the original JMdict, which — unlike the jmdict-simplified
// build — still carries the ke_pri/re_pri priority markers behind freq_rank.
// It is published on its own, outside the GitHub release.
const jmdictXMLURL = "https://www.edrdg.org/pub/Nihongo/JMdict_e.gz"

var forceUpdate bool

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Download and import Japanese dictionaries and English-to-Thai data",
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
	// Must come after Kanjidic2: kanji_radicals has a foreign key on kanji(literal).
	{prefix: "kradfile-", suffix: ".json.zip", importerF: func() importer.Importer { return importer.KradfileImporter{} }, label: "KRADFILE (radicals)"},
}

func runUpdate(cmd *cobra.Command, args []string) error {
	// Each store commits independently. A failure in one source must not stop
	// the other from updating, and an up-to-date Japanese store still checks Thai.
	var errs []error
	if err := runJapaneseUpdate(cmd, args); err != nil {
		errs = append(errs, fmt.Errorf("Japanese update: %w", err))
	}
	if err := cmd.Context().Err(); err != nil {
		return errors.Join(append(errs, err)...)
	}
	if err := updateThai(cmd, ""); err != nil {
		errs = append(errs, fmt.Errorf("EN→TH update: %w", err))
	}
	return errors.Join(errs...)
}

func runJapaneseUpdate(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	fetcher := source.NewGithubFetcher()

	fmt.Println("Checking latest release…")
	rel, err := fetcher.LatestRelease()
	if err != nil {
		return fmt.Errorf("fetch release info: %w", err)
	}
	fmt.Printf("Latest version: %s\n", rel.Version)

	// Check current versions in DB (if it exists). JMdict XML is versioned by
	// its Last-Modified header; a change in either source triggers a rebuild.
	if !forceUpdate {
		current, err := currentVersion(resolveDBPath(), "jmdict_version")
		currentPriority, priorityErr := currentVersion(resolveDBPath(), "jmdict_priority_version")
		remotePriority, remoteErr := fetcher.RemoteVersion(jmdictXMLURL)
		priorityCurrent := priorityErr == nil && remoteErr == nil && currentPriority == remotePriority
		if err == nil && current == rel.Version && priorityCurrent {
			fmt.Println("Japanese dictionaries already up to date.")
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

	// Frequency ranking is a nice-to-have: without it search still works, just
	// ordered less well, so a failure here must not sink the whole update.
	if err := importJMdictPriority(ctx, fetcher, tmpDB); err != nil {
		fmt.Printf("\n  WARNING: could not import frequency ranking (%v).\n", err)
		fmt.Println("  Search results will be ordered without it.")
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

// importJMdictPriority downloads the original JMdict XML and records the
// per-entry frequency rank. It runs after the JMdict import, since it updates
// word rows that must already exist.
func importJMdictPriority(ctx context.Context, fetcher source.Fetcher, db *sql.DB) error {
	fmt.Println("\nDownloading JMdict priority data (JMdict_e.gz)…")
	// Size -1 renders a spinner: there is no release metadata to read it from,
	// and the importer records the version itself from the file header.
	asset := &source.Asset{Name: "JMdict_e.gz", DownloadURL: jmdictXMLURL, Size: -1}
	data, err := downloadToMemory(ctx, fetcher, asset)
	if err != nil {
		return err
	}

	fmt.Println("Importing frequency ranking…")
	// The bar tracks the compressed bytes, since the decompressed size is
	// unknown until the whole stream has been read.
	bar := progressbar.DefaultBytes(int64(len(data)), "  importing")
	reader := progressbar.NewReader(bytes.NewReader(data), bar)
	gz, err := gzip.NewReader(&reader)
	if err != nil {
		bar.Finish()
		return fmt.Errorf("open gzip: %w", err)
	}
	defer gz.Close()

	err = importer.JMdictPriorityImporter{}.Import(ctx, db, gz, 0, nil)
	bar.Finish()
	if err != nil {
		return err
	}

	var ranked int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM words WHERE freq_rank IS NOT NULL`).Scan(&ranked); err != nil {
		return err
	}
	if ranked == 0 {
		return fmt.Errorf("no entries were ranked")
	}
	fmt.Printf("\nRanked %d words by frequency.\n", ranked)
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

// currentVersion reads a source_meta value from the existing database, if any.
func currentVersion(path, key string) (string, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return "", err
	}
	d, err := sql.Open("sqlite", path)
	if err != nil {
		return "", err
	}
	defer d.Close()
	var v string
	err = d.QueryRow(`SELECT value FROM source_meta WHERE key=?`, key).Scan(&v)
	return v, err
}

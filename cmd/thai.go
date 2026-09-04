package cmd

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	jishodb "github.com/shsnail/jisho/internal/db"
	"github.com/shsnail/jisho/internal/importer"
	"github.com/shsnail/jisho/internal/model"
	"github.com/shsnail/jisho/internal/output"
	"github.com/shsnail/jisho/internal/query"
	"github.com/shsnail/jisho/internal/source"
	"github.com/spf13/cobra"
)

const lexitronAcknowledgement = "This product is created by the adaptation of LEXiTRON developed by NECTEC (http://www.nectec.or.th/)."

func init() {
	lookup := &cobra.Command{
		Use:   "th <English word or phrase>",
		Short: "Look up an English word or phrase in the offline Thai dictionary",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return lookupThaiTo(cmd.Context(), cmd.OutOrStdout(), strings.Join(args, " "))
		},
	}
	update := &cobra.Command{
		Use:   "update-th",
		Short: "Download and import NECTEC's LEXiTRON English-to-Thai dictionary",
		Args:  cobra.NoArgs,
		RunE:  runUpdateThai,
	}
	update.Flags().String("file", "", "Import an existing official LEXiTRON 2.0 ZIP archive")
	rootCmd.AddCommand(lookup, update)
}

func resolveThaiDBPath() string {
	main := resolveDBPath()
	return strings.TrimSuffix(main, filepath.Ext(main)) + "-en-th.db"
}

func lookupThaiTo(ctx context.Context, w io.Writer, text string) error {
	entries, err := lookupThaiEntries(ctx, text)
	if err != nil {
		return err
	}
	output.PrintThai(w, text, entries)
	return nil
}

func lookupThaiEntries(ctx context.Context, text string) ([]model.ThaiEntry, error) {
	p := resolveThaiDBPath()
	if _, err := os.Stat(p); os.IsNotExist(err) {
		return nil, fmt.Errorf("Thai dictionary not installed; run `jisho update`")
	} else if err != nil {
		return nil, err
	}
	// Read-only mode avoids creating an empty store if it disappears after Stat.
	u := url.URL{Scheme: "file", Path: p, RawQuery: "mode=ro"}
	d, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, err
	}
	defer d.Close()
	entries, err := query.LookupThai(ctx, d, text)
	if err != nil {
		return nil, fmt.Errorf("Thai lookup: %w", err)
	}
	return entries, nil
}

func runUpdateThai(cmd *cobra.Command, _ []string) error {
	file, _ := cmd.Flags().GetString("file")
	return updateThai(cmd, file)
}

func updateThai(cmd *cobra.Command, file string) error {
	var data []byte
	var err error
	if file != "" {
		data, err = os.ReadFile(file)
	} else {
		fmt.Fprintln(cmd.OutOrStdout(), "Downloading LEXiTRON 2.0 from NECTEC…")
		data, err = source.DownloadLexitron(cmd.Context())
	}
	if err != nil {
		return err
	}
	count, err := installThaiArchive(cmd.Context(), resolveThaiDBPath(), data)
	if err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Imported %d EN→TH senses into %s\n%s\n", count, resolveThaiDBPath(), lexitronAcknowledgement)
	return nil
}

// installThaiArchive validates and builds a new store before atomically replacing
// the previous one. The licenses and source checksum travel inside the database.
func installThaiArchive(ctx context.Context, dest string, data []byte) (int, error) {
	z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return 0, fmt.Errorf("LEXiTRON ZIP: %w", err)
	}
	files := make(map[string][]byte)
	for _, f := range z.File {
		name := path.Base(f.Name)
		if name != "etlex.csv" && name != "LICENSE.txt" && name != "LICENSE-th.txt" {
			continue
		}
		if _, exists := files[name]; exists {
			return 0, fmt.Errorf("duplicate archive entry: %s", name)
		}
		r, err := f.Open()
		if err != nil {
			return 0, err
		}
		const maxSize = 64 << 20
		b, readErr := io.ReadAll(io.LimitReader(r, maxSize+1))
		r.Close()
		if readErr != nil {
			return 0, readErr
		}
		if len(b) > maxSize {
			return 0, fmt.Errorf("archive entry %s exceeds 64 MiB", name)
		}
		files[name] = b
	}
	for _, name := range []string{"etlex.csv", "LICENSE.txt", "LICENSE-th.txt"} {
		if len(files[name]) == 0 {
			return 0, fmt.Errorf("LEXiTRON archive missing %s", name)
		}
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return 0, err
	}
	f, err := os.CreateTemp(filepath.Dir(dest), ".jisho-thai-*.db")
	if err != nil {
		return 0, err
	}
	tmp := f.Name()
	f.Close()
	defer os.Remove(tmp)
	d, err := jishodb.OpenThai(tmp)
	if err != nil {
		return 0, err
	}
	defer d.Close()
	csvData := files["etlex.csv"]
	if err := (importer.LexitronImporter{}).Import(ctx, d, bytes.NewReader(csvData), int64(len(csvData)), nil); err != nil {
		return 0, err
	}
	meta := map[string]string{
		"lexitron_source_url":      source.LexitronURL,
		"lexitron_sha256":          fmt.Sprintf("%x", sha256.Sum256(data)),
		"lexitron_license":         string(files["LICENSE.txt"]),
		"lexitron_license_th":      string(files["LICENSE-th.txt"]),
		"lexitron_acknowledgement": lexitronAcknowledgement,
	}
	for key, value := range meta {
		if _, err := d.ExecContext(ctx, `INSERT INTO source_meta (key, value, updated_at) VALUES (?, ?, datetime('now'))`, key, value); err != nil {
			return 0, err
		}
	}
	var count int
	if err := d.QueryRowContext(ctx, `SELECT count(*) FROM thai_entries`).Scan(&count); err != nil {
		return 0, err
	}
	if err := d.Close(); err != nil {
		return 0, err
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if err := os.Rename(tmp, dest); err != nil {
		return 0, err
	}
	return count, nil
}

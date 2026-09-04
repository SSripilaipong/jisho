package cmd

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	jishodb "github.com/shsnail/jisho/internal/db"
	"github.com/shsnail/jisho/internal/source"
	"github.com/spf13/cobra"
)

type updateTransport func(*http.Request) (*http.Response, error)

func (f updateTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestUpdateRefreshesBothDictionaries(t *testing.T) {
	for _, tc := range []struct {
		name          string
		japaneseFails bool
		thaiFails     bool
		wantError     string
	}{
		{name: "Japanese already current still installs Thai"},
		{name: "Japanese failure still installs Thai", japaneseFails: true, wantError: "Japanese update:"},
		{name: "Thai failure preserves installed data", thaiFails: true, wantError: "EN→TH update:"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			oldPath, oldForce, oldTransport := dbPath, forceUpdate, http.DefaultTransport
			t.Cleanup(func() {
				dbPath, forceUpdate, http.DefaultTransport = oldPath, oldForce, oldTransport
			})
			dbPath, forceUpdate = filepath.Join(t.TempDir(), "jisho.db"), false
			// Only version metadata is needed for the Japanese up-to-date path.
			d, err := jishodb.OpenThai(dbPath)
			if err != nil {
				t.Fatal(err)
			}
			_, err = d.Exec(`INSERT INTO source_meta (key, value, updated_at) VALUES
				('jmdict_version', 'test-release', 'now'),
				('jmdict_priority_version', 'test-priority', 'now')`)
			d.Close()
			if err != nil {
				t.Fatal(err)
			}
			archive := thaiArchive(t, thaiCSV, true)
			var before []byte
			if tc.thaiFails {
				if _, err := installThaiArchive(context.Background(), resolveThaiDBPath(), archive); err != nil {
					t.Fatal(err)
				}
				before, err = os.ReadFile(resolveThaiDBPath())
				if err != nil {
					t.Fatal(err)
				}
			}
			thaiRequests := 0
			http.DefaultTransport = updateTransport(func(r *http.Request) (*http.Response, error) {
				status, body := http.StatusOK, []byte(nil)
				header := make(http.Header)
				switch r.URL.String() {
				case "https://api.github.com/repos/scriptin/jmdict-simplified/releases/latest":
					body = []byte(`{"tag_name":"test-release","assets":[]}`)
					if tc.japaneseFails {
						status = http.StatusServiceUnavailable
					}
				case jmdictXMLURL:
					if r.Method != http.MethodHead {
						t.Fatalf("unexpected Japanese download: %s", r.Method)
					}
					header.Set("Last-Modified", "test-priority")
				case source.LexitronURL:
					thaiRequests++
					body = archive
					if tc.thaiFails {
						status = http.StatusServiceUnavailable
					}
				default:
					t.Fatalf("unexpected request: %s", r.URL)
				}
				return &http.Response{StatusCode: status, Header: header, Body: io.NopCloser(bytes.NewReader(body)), Request: r}, nil
			})
			cmd := &cobra.Command{}
			cmd.SetContext(context.Background())
			cmd.SetOut(io.Discard)
			err = updateCmd.RunE(cmd, nil)
			if tc.wantError == "" && err != nil {
				t.Fatal(err)
			}
			if tc.wantError != "" && (err == nil || !strings.Contains(err.Error(), tc.wantError)) {
				t.Fatalf("error=%v, want %q", err, tc.wantError)
			}
			if thaiRequests != 1 {
				t.Fatalf("Thai downloads=%d, want 1", thaiRequests)
			}
			var out bytes.Buffer
			if err := lookupThaiTo(context.Background(), &out, "bank"); err != nil || !strings.Contains(out.String(), "ธนาคาร") {
				t.Fatalf("Thai lookup=%q, error=%v", out.String(), err)
			}
			if tc.thaiFails {
				after, err := os.ReadFile(resolveThaiDBPath())
				if err != nil || !bytes.Equal(before, after) {
					t.Fatalf("failed refresh changed database: %v", err)
				}
			}
		})
	}
}

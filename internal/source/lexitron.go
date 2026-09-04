package source

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

const LexitronURL = "https://opend-portal.nectec.or.th/dataset/bdd85296-9398-499f-b3a7-aab85042d3f9/resource/55683ab0-4f68-495a-b1bd-0783fafa9cbc/download/lexitron_2.0_csv.zip"

func DownloadLexitron(ctx context.Context) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, LexitronURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	client := &http.Client{Timeout: 2 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("LEXiTRON download returned HTTP %d", resp.StatusCode)
	}
	const maxSize = 32 << 20
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxSize+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxSize {
		return nil, fmt.Errorf("LEXiTRON archive exceeds 32 MiB")
	}
	return data, nil
}

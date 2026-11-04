package streaming

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"testing"
	"time"
)

type countedResponseBody struct {
	io.ReadCloser
	bytes *int64
}

func (b countedResponseBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	*b.bytes += int64(n)
	return n, err
}

// Public HTML only. Evidence contains counters and fixed categories, never
// response bodies, visitor identifiers, headers, URLs or credentials.
func TestProofHomeResponseLive(t *testing.T) {
	if os.Getenv("YTMGO_LIVE_PROOF_HOME") != "1" {
		t.Skip("explicit public HTML validation not selected")
	}
	transport := transportClient(nil)
	defer transport.CloseIdleConnections()
	var row struct {
		Category     string `json:"category"`
		Status       int    `json:"status"`
		Gzip         bool   `json:"gzip"`
		WireBytes    int64  `json:"wire_bytes"`
		DecodedBytes int    `json:"decoded_bytes"`
		ElapsedMS    int64  `json:"elapsed_ms"`
		Parsed       bool   `json:"proof_page_parsed"`
	}
	r, err := New(Options{Command: func(context.Context, string) *exec.Cmd { t.Fatal("unexpected helper"); return nil }, Client: &http.Client{Transport: roundTrip(func(req *http.Request) (*http.Response, error) {
		res, err := transport.Do(req)
		if res != nil {
			row.Status, row.Gzip = res.StatusCode, res.Header.Get("Content-Encoding") == "gzip"
			res.Body = countedResponseBody{res.Body, &row.WireBytes}
		}
		return res, err
	})}})
	if err != nil {
		t.Fatal("setup failed")
	}
	defer r.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 65*time.Second)
	defer cancel()
	started := time.Now()
	data, err := r.request(ctx, "GET", "https://www.youtube.com/", nil, http.Header{"Accept": {"text/html"}, "Accept-Language": {"en-US,en;q=0.7"}}, "proof_init", "")
	row.ElapsedMS, row.DecodedBytes = time.Since(started).Milliseconds(), len(data)
	row.Category = "request_failed"
	if err == nil {
		_, err = parseProofPage(data)
		row.Parsed = err == nil
		if row.Parsed {
			row.Category = "pass"
		} else {
			row.Category = "parse_failed"
		}
	}
	if path := os.Getenv("YTMGO_LIVE_EVIDENCE"); path != "" {
		encoded, _ := json.MarshalIndent(row, "", "  ")
		if os.WriteFile(path, append(encoded, '\n'), 0600) != nil {
			t.Fatal("evidence write failed")
		}
	}
	if !row.Parsed {
		t.Fatal("production proof HTML request failed")
	}
}

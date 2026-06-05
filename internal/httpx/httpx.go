// Package httpx holds the shared HTTP+JSON fetch helper used by the data
// clients, so the User-Agent, response-size limit, and decode policy live in
// one place.
package httpx

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const (
	userAgent = "thuisbord/1.0 (home dashboard)"
	maxBody   = 1 << 20 // 1 MiB is plenty for these JSON responses
)

// GetJSON issues a GET to url and decodes the JSON body into v.
func GetJSON(ctx context.Context, client *http.Client, url string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return err
	}
	return json.Unmarshal(body, v)
}

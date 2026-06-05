// Package ovapi fetches real-time bus departures from OVapi's KV78turbo feed
// (http://v0.ovapi.nl/tpc/{code}). This is the same DRIS data that drives the
// physical departure boards at Dutch stops, so ExpectedDepartureTime matches
// what you see on the board, including live delays.
//
// Note: OVapi must be called over plain http:// — its TLS certificate is only
// valid for the apex ovapi.nl, so https on the v0. host fails.
package ovapi

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

const baseURL = "http://v0.ovapi.nl/tpc/"

// ovapi timestamps look like "2026-06-05T17:42:00" with no zone — they are
// local Europe/Amsterdam, so we parse them in the configured location.
const timeLayout = "2006-01-02T15:04:05"

// Departure is one upcoming bus at a stop.
type Departure struct {
	Line        string    // public line number, e.g. "305"
	Destination string    // headsign, e.g. "Amsterdam Centraal"
	Target      time.Time // scheduled departure
	Expected    time.Time // live departure (use this — matches the board)
}

// Stop is a single quay with its sorted upcoming departures.
type Stop struct {
	Code       string
	Name       string
	Departures []Departure
}

// Board is the full set of stops fetched in one request.
type Board struct {
	Stops []Stop
}

// Client fetches departures for a fixed set of TimingPointCodes.
type Client struct {
	http *http.Client
	loc  *time.Location
}

// New returns a Client rendering times in loc.
func New(loc *time.Location) *Client {
	return &Client{
		http: &http.Client{Timeout: 8 * time.Second},
		loc:  loc,
	}
}

// raw mirrors the OVapi JSON shape: an object keyed by TimingPointCode, each
// holding a Stop block and a Passes object (a map, not an array).
type rawStop struct {
	Stop struct {
		TimingPointName string `json:"TimingPointName"`
		TimingPointCode string `json:"TimingPointCode"`
	} `json:"Stop"`
	Passes map[string]rawPass `json:"Passes"`
}

type rawPass struct {
	LinePublicNumber      string `json:"LinePublicNumber"`
	DestinationName50     string `json:"DestinationName50"`
	TransportType         string `json:"TransportType"`
	TripStopStatus        string `json:"TripStopStatus"`
	TargetDepartureTime   string `json:"TargetDepartureTime"`
	ExpectedDepartureTime string `json:"ExpectedDepartureTime"`
}

// Fetch retrieves and normalises departures for the given codes (one HTTP call;
// OVapi supports a comma-separated multi-code request). Stops are returned in
// the same order as codes; PASSED departures are dropped and each stop's
// departures are sorted by expected time.
func (c *Client) Fetch(ctx context.Context, codes []string) (Board, error) {
	if len(codes) == 0 {
		return Board{}, nil
	}
	url := baseURL + strings.Join(codes, ",")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Board{}, err
	}
	req.Header.Set("User-Agent", "thuisbord/1.0 (home dashboard)")
	req.Header.Set("Accept-Encoding", "gzip")

	resp, err := c.http.Do(req)
	if err != nil {
		return Board{}, fmt.Errorf("ovapi request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Board{}, fmt.Errorf("ovapi status %d", resp.StatusCode)
	}

	// We request gzip explicitly, so Go won't transparently decompress —
	// handle it ourselves based on the response's Content-Encoding.
	reader := io.Reader(resp.Body)
	if strings.EqualFold(resp.Header.Get("Content-Encoding"), "gzip") {
		gz, err := gzip.NewReader(resp.Body)
		if err != nil {
			return Board{}, fmt.Errorf("ovapi gzip: %w", err)
		}
		defer gz.Close()
		reader = gz
	}
	body, err := io.ReadAll(io.LimitReader(reader, 16<<20))
	if err != nil {
		return Board{}, fmt.Errorf("ovapi read: %w", err)
	}

	var raw map[string]rawStop
	if err := json.Unmarshal(body, &raw); err != nil {
		return Board{}, fmt.Errorf("ovapi decode: %w", err)
	}

	var board Board
	for _, code := range codes {
		rs, present := raw[code]
		if !present {
			continue
		}
		stop := Stop{Code: code, Name: rs.Stop.TimingPointName}
		for _, p := range rs.Passes {
			if strings.EqualFold(p.TripStopStatus, "PASSED") {
				continue
			}
			expected := c.parse(p.ExpectedDepartureTime)
			if expected.IsZero() {
				continue
			}
			stop.Departures = append(stop.Departures, Departure{
				Line:        p.LinePublicNumber,
				Destination: p.DestinationName50,
				Target:      c.parse(p.TargetDepartureTime),
				Expected:    expected,
			})
		}
		sort.Slice(stop.Departures, func(i, j int) bool {
			return stop.Departures[i].Expected.Before(stop.Departures[j].Expected)
		})
		board.Stops = append(board.Stops, stop)
	}
	return board, nil
}

func (c *Client) parse(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.ParseInLocation(timeLayout, s, c.loc)
	if err != nil {
		return time.Time{}
	}
	return t
}

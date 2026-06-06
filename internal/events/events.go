// Package events fetches local events from the DagjeWeg.NL RSS feed (zero-auth)
// and filters them to a postcode prefix (Purmerend = "144"). The feed is
// national; each item carries the postcode at the start of its description and
// the date in its title, both of which we parse out.
package events

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const feedURL = "https://www.dagjeweg.nl/rss.kalender.php"

// Event is a single upcoming local event.
type Event struct {
	Date  time.Time
	Title string
}

// Client fetches and filters the feed for one postcode prefix.
type Client struct {
	http   *http.Client
	loc    *time.Location
	prefix string // e.g. "144"
}

// New returns a Client filtering to the given postcode prefix.
func New(loc *time.Location, prefix string) *Client {
	return &Client{http: &http.Client{Timeout: 8 * time.Second}, loc: loc, prefix: prefix}
}

type rssFeed struct {
	Items []struct {
		Title       string `xml:"title"`
		Description string `xml:"description"`
	} `xml:"channel>item"`
}

var (
	postcodeRe = regexp.MustCompile(`^\s*(\d{4})`)
	dateRe     = regexp.MustCompile(`(\d{1,2})\s+(januari|februari|maart|april|mei|juni|juli|augustus|september|oktober|november|december)\s+(\d{4})`)
)

var dutchMonth = map[string]time.Month{
	"januari": 1, "februari": 2, "maart": 3, "april": 4, "mei": 5, "juni": 6,
	"juli": 7, "augustus": 8, "september": 9, "oktober": 10, "november": 11, "december": 12,
}

// Fetch retrieves the feed and returns events matching the postcode prefix,
// sorted by date.
func (c *Client) Fetch(ctx context.Context) ([]Event, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, feedURL, nil)
	if err != nil {
		return nil, err
	}
	// DagjeWeg blocks unknown agents; present a browser-like one.
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; thuisbord/1.0)")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("dagjeweg request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("dagjeweg status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}

	var feed rssFeed
	if err := xml.Unmarshal(body, &feed); err != nil {
		return nil, fmt.Errorf("dagjeweg decode: %w", err)
	}

	var out []Event
	for _, it := range feed.Items {
		pc := postcodeRe.FindStringSubmatch(it.Description)
		if pc == nil || !strings.HasPrefix(pc[1], c.prefix) {
			continue
		}
		date, ok := c.parseTitleDate(it.Title)
		if !ok {
			continue
		}
		out = append(out, Event{Date: date, Title: cleanTitle(it.Title)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Date.Before(out[j].Date) })
	return out, nil
}

// parseTitleDate extracts the first "d maand yyyy" date from the title.
func (c *Client) parseTitleDate(title string) (time.Time, bool) {
	m := dateRe.FindStringSubmatch(title)
	if m == nil {
		return time.Time{}, false
	}
	day, _ := strconv.Atoi(m[1])
	mon := dutchMonth[m[2]]
	year, _ := strconv.Atoi(m[3])
	return time.Date(year, mon, day, 0, 0, 0, 0, c.loc), true
}

// cleanTitle strips the trailing date portion and a trailing place suffix.
func cleanTitle(title string) string {
	if loc := dateRe.FindStringIndex(title); loc != nil {
		title = title[:loc[0]]
	}
	title = strings.TrimSpace(title)
	title = strings.TrimSuffix(title, " in Purmerend")
	title = strings.TrimSuffix(title, " Purmerend")
	return strings.TrimSpace(title)
}

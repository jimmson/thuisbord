// Package holidays fetches Dutch public holidays from the Nager.Date API
// (zero-auth, https://date.nager.at). Used to mark feestdagen on the calendar.
package holidays

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"time"

	"thuisbord/internal/httpx"
)

const apiBase = "https://date.nager.at/api/v3/PublicHolidays"

// Holiday is one public holiday.
type Holiday struct {
	Date time.Time
	Name string // Dutch local name, e.g. "Tweede Pinksterdag"
}

// Client fetches NL holidays.
type Client struct {
	http *http.Client
	loc  *time.Location
}

// New returns a holidays client rendering dates in loc.
func New(loc *time.Location) *Client {
	return &Client{http: &http.Client{Timeout: 8 * time.Second}, loc: loc}
}

type rawHoliday struct {
	Date      string `json:"date"` // "2026-12-25"
	LocalName string `json:"localName"`
}

// Fetch returns this year's and next year's NL holidays, sorted by date, so the
// calendar window and the upcoming list are covered across the year boundary.
func (c *Client) Fetch(ctx context.Context) ([]Holiday, error) {
	now := time.Now().In(c.loc)
	var out []Holiday
	for _, year := range []int{now.Year(), now.Year() + 1} {
		var raw []rawHoliday
		url := fmt.Sprintf("%s/%d/NL", apiBase, year)
		if err := httpx.GetJSON(ctx, c.http, url, &raw); err != nil {
			return nil, fmt.Errorf("holidays %d: %w", year, err)
		}
		for _, h := range raw {
			d, err := time.ParseInLocation("2006-01-02", h.Date, c.loc)
			if err != nil {
				continue
			}
			out = append(out, Holiday{Date: d, Name: h.LocalName})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Date.Before(out[j].Date) })
	return out, nil
}

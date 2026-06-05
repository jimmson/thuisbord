// Package opzet fetches the household waste collection schedule from the Opzet
// platform that powers Purmerend's afvalkalender. Two unauthenticated JSON
// calls: resolve a stable bagId from postcode+house number (once), then fetch
// the waste streams (afvalstromen) with the next collection date per fraction.
package opzet

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"thuisbord/internal/httpx"
)

const baseURL = "https://afvalkalender.purmerend.nl"

// dateLayout is the ophaaldatum format, e.g. "2026-06-18".
const dateLayout = "2006-01-02"

// Pickup is one waste fraction with its next collection date.
type Pickup struct {
	Fraction string    // raw menu_title, e.g. "Bio-afval"
	Kind     string    // normalised key for styling: rest|organic|paper|plastic|glass|textile|other
	Date     time.Time // next collection (date only, in the configured location)
}

// Client fetches the schedule for a single address.
type Client struct {
	http        *http.Client
	loc         *time.Location
	postcode    string
	houseNumber string

	mu    sync.Mutex
	bagID string // cached after first resolution; stable forever
}

// New returns a Client for the given address.
func New(loc *time.Location, postcode, houseNumber string) *Client {
	return &Client{
		http:        &http.Client{Timeout: 8 * time.Second},
		loc:         loc,
		postcode:    postcode,
		houseNumber: houseNumber,
	}
}

type rawAddress struct {
	BagID                string `json:"bagId"`
	Huisnummer           int    `json:"huisnummer"`
	HuisnummerToevoeging string `json:"huisnummerToevoeging"`
	Huisletter           string `json:"huisletter"`
}

type rawStream struct {
	ID        int    `json:"id"`
	MenuTitle string `json:"menu_title"`
}

type rawCalEntry struct {
	AfvalstroomID int    `json:"afvalstroom_id"`
	Ophaaldatum   string `json:"ophaaldatum"`
}

// Fetch resolves the bagId (cached) then returns every scheduled pickup for the
// current year (and the next year, when the window can cross into it), sorted by
// date. It joins the year calendar (dates per afvalstroom_id) with the
// afvalstromen list (id -> fraction name). The view layer windows this down to
// what it displays.
func (c *Client) Fetch(ctx context.Context) ([]Pickup, error) {
	bagID, err := c.resolveBagID(ctx)
	if err != nil {
		return nil, err
	}

	// id -> fraction name/kind, from the afvalstromen list.
	var streams []rawStream
	streamsURL := fmt.Sprintf("%s/rest/adressen/%s/afvalstromen", baseURL, bagID)
	if err := httpx.GetJSON(ctx, c.http, streamsURL, &streams); err != nil {
		return nil, fmt.Errorf("afvalstromen: %w", err)
	}
	type meta struct{ fraction, kind string }
	byID := make(map[int]meta, len(streams))
	for _, s := range streams {
		byID[s.ID] = meta{fraction: s.MenuTitle, kind: classify(s.MenuTitle)}
	}

	// Full collection calendar for the relevant year(s).
	now := time.Now().In(c.loc)
	years := []int{now.Year()}
	if end := now.AddDate(0, 0, 35).Year(); end != now.Year() {
		years = append(years, end) // window can spill into next year
	}

	var pickups []Pickup
	for _, year := range years {
		var entries []rawCalEntry
		url := fmt.Sprintf("%s/rest/adressen/%s/kalender/%d", baseURL, bagID, year)
		if err := httpx.GetJSON(ctx, c.http, url, &entries); err != nil {
			return nil, fmt.Errorf("kalender %d: %w", year, err)
		}
		for _, e := range entries {
			m, ok := byID[e.AfvalstroomID]
			if !ok || e.Ophaaldatum == "" {
				continue
			}
			d, err := time.ParseInLocation(dateLayout, e.Ophaaldatum, c.loc)
			if err != nil {
				continue
			}
			pickups = append(pickups, Pickup{Fraction: m.fraction, Kind: m.kind, Date: d})
		}
	}
	sort.Slice(pickups, func(i, j int) bool { return pickups[i].Date.Before(pickups[j].Date) })
	return pickups, nil
}

func (c *Client) resolveBagID(ctx context.Context) (string, error) {
	c.mu.Lock()
	cached := c.bagID
	c.mu.Unlock()
	if cached != "" {
		return cached, nil
	}

	url := fmt.Sprintf("%s/rest/adressen/%s-%s", baseURL, c.postcode, c.houseNumber)
	var addrs []rawAddress
	if err := httpx.GetJSON(ctx, c.http, url, &addrs); err != nil {
		return "", fmt.Errorf("address lookup: %w", err)
	}
	if len(addrs) == 0 {
		return "", fmt.Errorf("no address found for %s-%s", c.postcode, c.houseNumber)
	}
	bagID := addrs[0].BagID // base house number; first match is correct for plain numbers

	c.mu.Lock()
	c.bagID = bagID
	c.mu.Unlock()
	return bagID, nil
}

// classify maps a Dutch fraction name to a stable styling key.
func classify(title string) string {
	t := strings.ToLower(title)
	switch {
	case strings.Contains(t, "rest"):
		return "rest"
	case strings.Contains(t, "bio") || strings.Contains(t, "gft") || strings.Contains(t, "groente"):
		return "organic"
	case strings.Contains(t, "papier") || strings.Contains(t, "karton"):
		return "paper"
	case strings.Contains(t, "plastic") || strings.Contains(t, "pmd") || strings.Contains(t, "blik"):
		return "plastic"
	case strings.Contains(t, "glas"):
		return "glass"
	case strings.Contains(t, "textiel"):
		return "textile"
	default:
		return "other"
	}
}

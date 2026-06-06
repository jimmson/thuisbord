// Package config loads Thuisbord's runtime configuration from environment
// variables. All deployment-specific values (bus stops, address, cadences)
// live here so the binary stays generic.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds every tunable for a single deployment.
type Config struct {
	Addr           string         // listen address, e.g. ":8080"
	BusStopCodes   []string       // OVapi TimingPointCodes (one per quay/direction)
	WalkOffset     time.Duration  // walking time from home to the stop
	Postcode       string         // e.g. "1011AB" (uppercase, no space)
	HouseNumber    string         // e.g. "1"
	Lat            float64        // latitude for weather
	Lon            float64        // longitude for weather
	RefreshSeconds int            // kiosk full-page auto-reload cadence
	BusPoll        time.Duration  // how often to refresh bus departures
	TrashPoll      time.Duration  // how often to refresh the waste schedule
	WeatherPoll    time.Duration  // how often to refresh the weather
	EventsPoll     time.Duration  // how often to refresh local events
	HolidaysPoll   time.Duration  // how often to refresh public holidays
	TrashWeeks     int            // number of weeks shown in the waste calendar
	MaxDepartures  int            // max bus departures shown
	Location       *time.Location // Europe/Amsterdam — drives all time rendering
}

// Load reads configuration from the environment, applying neutral example
// defaults (override everything via env vars / a .env file — see .env.example).
// It fails fast on an invalid timezone so a misconfigured image never silently
// renders in UTC.
func Load() (Config, error) {
	loc, err := time.LoadLocation(getenv("TZ", "Europe/Amsterdam"))
	if err != nil {
		return Config{}, fmt.Errorf("invalid TZ %q (is time/tzdata imported?): %w", os.Getenv("TZ"), err)
	}

	return Config{
		Addr:           getenv("DASH_ADDR", ":8080"),
		BusStopCodes:   splitCSV(getenv("DASH_BUS_TPC", "37400110")), // example: an OVapi TimingPointCode
		WalkOffset:     time.Duration(atoi(getenv("DASH_WALK_OFFSET_MIN", "8"), 8)) * time.Minute,
		Postcode:       strings.ToUpper(strings.ReplaceAll(getenv("DASH_POSTCODE", "1011AB"), " ", "")),
		HouseNumber:    getenv("DASH_HOUSE_NUMBER", "1"),
		Lat:            atof(getenv("DASH_LAT", "52.3731"), 52.3731), // example: Amsterdam
		Lon:            atof(getenv("DASH_LON", "4.8922"), 4.8922),
		RefreshSeconds: atoi(getenv("DASH_REFRESH_SECONDS", "60"), 60),
		BusPoll:        time.Duration(atoi(getenv("DASH_BUS_POLL_SECONDS", "30"), 30)) * time.Second,
		TrashPoll:      time.Duration(atoi(getenv("DASH_TRASH_POLL_HOURS", "6"), 6)) * time.Hour,
		WeatherPoll:    time.Duration(atoi(getenv("DASH_WEATHER_POLL_MIN", "15"), 15)) * time.Minute,
		EventsPoll:     time.Duration(atoi(getenv("DASH_EVENTS_POLL_HOURS", "6"), 6)) * time.Hour,
		HolidaysPoll:   time.Duration(atoi(getenv("DASH_HOLIDAYS_POLL_HOURS", "24"), 24)) * time.Hour,
		TrashWeeks:     atoi(getenv("DASH_TRASH_WEEKS", "4"), 4),
		MaxDepartures:  atoi(getenv("DASH_MAX_DEPARTURES", "4"), 4),
		Location:       loc,
	}, nil
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func atoi(s string, def int) int {
	if n, err := strconv.Atoi(strings.TrimSpace(s)); err == nil {
		return n
	}
	return def
}

func atof(s string, def float64) float64 {
	if f, err := strconv.ParseFloat(strings.TrimSpace(s), 64); err == nil {
		return f
	}
	return def
}

func splitCSV(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}

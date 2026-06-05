// Package weather fetches a small forecast from Open-Meteo (free, no API key):
// current conditions, a +3h outlook, and the next two days. WMO weather codes
// are mapped to Lucide icon names and short Dutch condition text.
package weather

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"time"

	"thuisbord/internal/httpx"
)

const apiBase = "https://api.open-meteo.com/v1/forecast"

// Current is the present conditions.
type Current struct {
	Temp int
	Cond string // Dutch, e.g. "Licht bewolkt"
	Icon string // Lucide icon name
}

// Outlook is the +3h forecast point.
type Outlook struct {
	Temp int
	Icon string
}

// Day is a future day's forecast; the view layer formats Date into a label.
type Day struct {
	Date time.Time
	Temp int
	Icon string
}

// Report is everything the weather header needs (no display strings — the view
// layer turns Day.Date into a weekday label).
type Report struct {
	Now       Current
	Plus3h    Outlook
	HasPlus3h bool
	Days      []Day
}

// Client fetches weather for a fixed location.
type Client struct {
	http *http.Client
	loc  *time.Location
	lat  float64
	lon  float64
}

// New returns a Client for the given coordinates, rendering times in loc.
func New(loc *time.Location, lat, lon float64) *Client {
	return &Client{http: &http.Client{Timeout: 8 * time.Second}, loc: loc, lat: lat, lon: lon}
}

type rawResp struct {
	Current struct {
		Time        string  `json:"time"`
		Temperature float64 `json:"temperature_2m"`
		WeatherCode int     `json:"weather_code"`
		IsDay       int     `json:"is_day"`
	} `json:"current"`
	Hourly struct {
		Time        []string  `json:"time"`
		Temperature []float64 `json:"temperature_2m"`
		WeatherCode []int     `json:"weather_code"`
		IsDay       []int     `json:"is_day"`
	} `json:"hourly"`
	Daily struct {
		Time        []string  `json:"time"`
		WeatherCode []int     `json:"weather_code"`
		TempMax     []float64 `json:"temperature_2m_max"`
	} `json:"daily"`
}

// Fetch retrieves and normalises the forecast.
func (c *Client) Fetch(ctx context.Context) (Report, error) {
	q := url.Values{}
	q.Set("latitude", fmt.Sprintf("%.4f", c.lat))
	q.Set("longitude", fmt.Sprintf("%.4f", c.lon))
	q.Set("current", "temperature_2m,weather_code,is_day")
	q.Set("hourly", "temperature_2m,weather_code,is_day")
	q.Set("daily", "weather_code,temperature_2m_max")
	q.Set("timezone", "Europe/Amsterdam")
	q.Set("forecast_days", "3")
	reqURL := apiBase + "?" + q.Encode()

	var raw rawResp
	if err := httpx.GetJSON(ctx, c.http, reqURL, &raw); err != nil {
		return Report{}, fmt.Errorf("open-meteo: %w", err)
	}

	now := time.Now().In(c.loc)

	rep := Report{}
	curIcon, curCond := describe(raw.Current.WeatherCode, raw.Current.IsDay == 1)
	rep.Now = Current{Temp: round(raw.Current.Temperature), Cond: curCond, Icon: curIcon}

	// +3h outlook: the hourly slot nearest to now+3h.
	if i := c.nearestHour(raw.Hourly.Time, now.Add(3*time.Hour)); i >= 0 && i < len(raw.Hourly.Temperature) {
		day := i < len(raw.Hourly.IsDay) && raw.Hourly.IsDay[i] == 1
		icon, _ := describe(raw.Hourly.WeatherCode[i], day)
		rep.Plus3h = Outlook{Temp: round(raw.Hourly.Temperature[i]), Icon: icon}
		rep.HasPlus3h = true
	}

	// Next two days: daily index 1 (tomorrow) and 2 (day after).
	for i := 1; i <= 2 && i < len(raw.Daily.Time); i++ {
		date, err := time.ParseInLocation("2006-01-02", raw.Daily.Time[i], c.loc)
		if err != nil {
			continue
		}
		icon, _ := describe(raw.Daily.WeatherCode[i], true)
		rep.Days = append(rep.Days, Day{
			Date: date,
			Temp: round(raw.Daily.TempMax[i]),
			Icon: icon,
		})
	}
	return rep, nil
}

// nearestHour returns the index of the hourly timestamp closest to target.
func (c *Client) nearestHour(times []string, target time.Time) int {
	best, bestDiff := -1, math.MaxFloat64
	for i, s := range times {
		t, err := time.ParseInLocation("2006-01-02T15:04", s, c.loc)
		if err != nil {
			continue
		}
		if d := math.Abs(t.Sub(target).Seconds()); d < bestDiff {
			best, bestDiff = i, d
		}
	}
	return best
}

func round(f float64) int { return int(math.Round(f)) }

// describe maps a WMO weather code to a Lucide icon name and short Dutch text.
// day controls sun/moon variants for clear and partly-cloudy conditions.
func describe(code int, day bool) (icon, text string) {
	switch code {
	case 0:
		if day {
			return "sun", "Helder"
		}
		return "moon", "Helder"
	case 1, 2:
		if day {
			return "cloud-sun", "Licht bewolkt"
		}
		return "cloud-moon", "Licht bewolkt"
	case 3:
		return "cloud", "Bewolkt"
	case 45, 48:
		return "cloud-fog", "Mist"
	case 51, 53, 55:
		return "cloud-drizzle", "Motregen"
	case 56, 57:
		return "cloud-drizzle", "IJzel"
	case 61, 63, 65:
		return "cloud-rain", "Regen"
	case 66, 67:
		return "cloud-rain", "IJzel"
	case 71, 73, 75, 77:
		return "cloud-snow", "Sneeuw"
	case 80, 81, 82:
		return "cloud-rain", "Buien"
	case 85, 86:
		return "cloud-snow", "Sneeuwbuien"
	case 95:
		return "cloud-lightning", "Onweer"
	case 96, 99:
		return "cloud-lightning", "Onweer met hagel"
	default:
		return "cloud", "Bewolkt"
	}
}

package web

import (
	"fmt"
	"sort"
	"time"

	"thuisbord/internal/events"
	"thuisbord/internal/holidays"
	"thuisbord/internal/opzet"
	"thuisbord/internal/ovapi"
	"thuisbord/internal/weather"
)

// ---- Weather ----

// WxSlot is a render-ready compact forecast point (label + icon + temp).
type WxSlot struct {
	Label string
	Icon  string
	Temp  int
}

// WeatherView is everything the header weather block needs.
type WeatherView struct {
	HasData bool
	Stale   bool
	Now     weather.Current
	Plus3h  *WxSlot // nil when no +3h outlook is available
	Days    []WxSlot
}

func buildWeatherView(r weather.Report, updatedAt time.Time, ok bool) WeatherView {
	v := WeatherView{
		HasData: ok,
		Stale:   !ok || time.Since(updatedAt) > 2*time.Hour,
		Now:     r.Now,
	}
	if r.HasPlus3h {
		v.Plus3h = &WxSlot{Label: "+3u", Icon: r.Plus3h.Icon, Temp: r.Plus3h.Temp}
	}
	for _, d := range r.Days {
		v.Days = append(v.Days, WxSlot{Label: weekdayShort(d.Date.Weekday()), Icon: d.Icon, Temp: d.Temp})
	}
	return v
}

// ---- Bus ----

// BusDeparture is one render-ready departure row.
type BusDeparture struct {
	Line         string
	Destination  string
	Time         string // "17:45"
	DelayMin     int    // minutes late (negative = early)
	MinutesUntil int    // minutes until the bus departs
	LeaveIn      int    // minutes until you must leave home (can be <= 0)
	LeaveNow     bool   // true when it's time to go
	Live         bool   // realtime-tracked (TripStopStatus DRIVING)
	Cancelled    bool   // trip cancelled
}

// BusView is everything the bus widget template needs.
type BusView struct {
	Stale      bool
	UpdatedAt  string
	StopName   string
	Departures []BusDeparture
	Empty      bool
}

func buildBusView(board ovapi.Board, loc *time.Location, walk time.Duration, max int, now time.Time, updatedAt time.Time, ok bool) BusView {
	if max < 1 {
		max = 4
	}
	walkMin := int(walk.Minutes())
	v := BusView{
		Stale:     !ok || time.Since(updatedAt) > 2*time.Minute,
		UpdatedAt: updatedAt.In(loc).Format("15:04"),
	}
	if len(board.Stops) > 0 {
		v.StopName = board.Stops[0].Name
	}

	// Merge departures across all configured stops, soonest first.
	var all []BusDeparture
	for _, s := range board.Stops {
		for _, d := range s.Departures {
			mins := int(d.Expected.Sub(now).Minutes())
			if mins < -1 {
				continue // already gone (1-minute grace so a just-now bus lingers)
			}
			if mins < 0 {
				mins = 0
			}
			delay := 0
			if !d.Target.IsZero() {
				delay = int(d.Expected.Sub(d.Target).Round(time.Minute).Minutes())
			}
			cancelled := d.Status == "CANCEL"
			leaveIn := mins - walkMin
			all = append(all, BusDeparture{
				Line:         d.Line,
				Destination:  d.Destination,
				Time:         d.Expected.In(loc).Format("15:04"),
				DelayMin:     delay,
				MinutesUntil: mins,
				LeaveIn:      leaveIn,
				LeaveNow:     !cancelled && leaveIn <= 0,
				Live:         d.Status == "DRIVING",
				Cancelled:    cancelled,
			})
		}
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].MinutesUntil < all[j].MinutesUntil })

	// Cap by real (non-cancelled) departures so cancellations never push an
	// actual upcoming bus off the board. Cancellations still show as context
	// (capped, so a burst of them can't bloat the widget).
	const maxCancelledShown = 2
	realKept, cancelledShown := 0, 0
	for _, dep := range all {
		if realKept >= max {
			break
		}
		if dep.Cancelled {
			if cancelledShown >= maxCancelledShown {
				continue
			}
			cancelledShown++
		} else {
			realKept++
		}
		v.Departures = append(v.Departures, dep)
	}
	v.Empty = len(v.Departures) == 0
	return v
}

// ---- Calendar (afval + events + holidays) ----

// DayMarker is one icon shown in a calendar day cell.
type DayMarker struct {
	Icon string // resolved Lucide icon name
	Kind string // colour/legend key: bin kinds, "event", "holiday"
}

// TrashDay is one cell in the month grid.
type TrashDay struct {
	DayNum  int
	Month   string // short month name, set only on the 1st of a month / first cell
	IsToday bool
	IsPast  bool
	InMonth bool // false for days of a different month (dimmed)
	Markers []DayMarker
}

// LegendItem keys the calendar icons below the grid.
type LegendItem struct {
	Icon  string
	Kind  string
	Label string
}

// TrashView is everything the calendar widget needs.
type TrashView struct {
	Stale     bool
	UpdatedAt string
	Days      []TrashDay   // weeks*7 cells, Monday-aligned
	Legend    []LegendItem // distinct marker kinds shown, as a key below the grid
}

// calItem is a unified calendar entry merged from every source. Adding a new
// source means producing more calItems — the grid, legend, and upcoming list
// all derive from these.
type calItem struct {
	Date  time.Time
	Title string
	Icon  string
	Kind  string
}

func mergeCalItems(pickups []opzet.Pickup, evs []events.Event, hols []holidays.Holiday) []calItem {
	items := make([]calItem, 0, len(pickups)+len(evs)+len(hols))
	for _, p := range pickups {
		items = append(items, calItem{Date: p.Date, Title: p.Fraction, Icon: binIcon(p.Kind), Kind: p.Kind})
	}
	for _, e := range evs {
		items = append(items, calItem{Date: e.Date, Title: e.Title, Icon: "party-popper", Kind: "event"})
	}
	for _, h := range hols {
		items = append(items, calItem{Date: h.Date, Title: h.Name, Icon: "flag", Kind: "holiday"})
	}
	return items
}

// legendLabel returns the key label for a kind (generic for non-bin sources).
func legendLabel(kind, title string) string {
	switch kind {
	case "event":
		return "Evenement"
	case "holiday":
		return "Feestdag"
	default:
		return title // bin fractions use their own name
	}
}

func buildTrashView(items []calItem, loc *time.Location, now, updatedAt time.Time, ok bool, weeks int) TrashView {
	if weeks < 1 {
		weeks = 4
	}
	today := dayStart(now)
	gridStart := startOfWeek(today)
	currentMonth := today.Month()

	v := TrashView{
		Stale:     !ok || time.Since(updatedAt) > 36*time.Hour,
		UpdatedAt: updatedAt.In(loc).Format("ma 2 jan 15:04"),
	}

	byDay := map[string][]calItem{}
	for _, it := range items {
		key := dayStart(it.Date).Format(dayKey)
		byDay[key] = append(byDay[key], it)
	}

	legendSeen := map[string]bool{}
	for i := 0; i < weeks*7; i++ {
		day := gridStart.AddDate(0, 0, i)
		cell := TrashDay{
			DayNum:  day.Day(),
			IsToday: day.Equal(today),
			IsPast:  day.Before(today),
			InMonth: day.Month() == currentMonth,
		}
		if day.Day() == 1 || i == 0 {
			cell.Month = monthShort(day.Month())
		}
		for _, it := range byDay[day.Format(dayKey)] {
			cell.Markers = append(cell.Markers, DayMarker{Icon: it.Icon, Kind: it.Kind})
			if !legendSeen[it.Kind] {
				legendSeen[it.Kind] = true
				v.Legend = append(v.Legend, LegendItem{Icon: it.Icon, Kind: it.Kind, Label: legendLabel(it.Kind, it.Title)})
			}
		}
		v.Days = append(v.Days, cell)
	}
	return v
}

// ---- Header "coming up" ----

// UpcomingItem is one entry in the header's next-things-up list.
type UpcomingItem struct {
	Icon  string
	Kind  string
	Title string
	When  string // "vandaag" / "morgen" / "do 18 jun"
}

func buildUpcoming(items []calItem, now time.Time, n int) []UpcomingItem {
	today := dayStart(now)
	future := make([]calItem, 0, len(items))
	for _, it := range items {
		if !dayStart(it.Date).Before(today) {
			future = append(future, it)
		}
	}
	sort.Slice(future, func(i, j int) bool { return future[i].Date.Before(future[j].Date) })
	out := make([]UpcomingItem, 0, n)
	for _, it := range future {
		if len(out) >= n {
			break
		}
		out = append(out, UpcomingItem{Icon: it.Icon, Kind: it.Kind, Title: it.Title, When: relativeDay(dayStart(it.Date), today)})
	}
	return out
}

const dayKey = "2006-01-02"

// ---- date helpers (Dutch) ----

func dayStart(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

// startOfWeek returns the Monday 00:00 of t's week.
func startOfWeek(t time.Time) time.Time {
	offset := (int(t.Weekday()) + 6) % 7 // Monday=0 .. Sunday=6
	return dayStart(t.AddDate(0, 0, -offset))
}

func relativeDay(d, today time.Time) string {
	switch days := int(d.Sub(today).Hours() / 24); days {
	case 0:
		return "vandaag"
	case 1:
		return "morgen"
	default:
		return fmt.Sprintf("%s %d %s", weekdayShort(d.Weekday()), d.Day(), monthShort(d.Month()))
	}
}

func weekdayShort(w time.Weekday) string {
	return [...]string{"zo", "ma", "di", "wo", "do", "vr", "za"}[w]
}

func monthShort(m time.Month) string {
	return [...]string{"", "jan", "feb", "mrt", "apr", "mei", "jun", "jul", "aug", "sep", "okt", "nov", "dec"}[m]
}

// dutchLongDate renders "vrijdag 5 juni" (the .date CSS capitalises each word).
func dutchLongDate(t time.Time) string {
	weekday := [...]string{"zondag", "maandag", "dinsdag", "woensdag", "donderdag", "vrijdag", "zaterdag"}[t.Weekday()]
	month := [...]string{"", "januari", "februari", "maart", "april", "mei", "juni", "juli", "augustus", "september", "oktober", "november", "december"}[t.Month()]
	return fmt.Sprintf("%s %d %s", weekday, t.Day(), month)
}

// binIcon maps a waste kind to a Lucide icon name.
func binIcon(kind string) string {
	switch kind {
	case "organic":
		return "leaf"
	case "paper":
		return "newspaper"
	case "plastic":
		return "recycle"
	case "glass":
		return "wine"
	case "textile":
		return "shirt"
	default: // rest, other
		return "trash-2"
	}
}

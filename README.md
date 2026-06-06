# Thuisbord

A small self-hosted **home dashboard** for a wall-mounted tablet. Server-rendered
HTML, auto-refreshing, styled in a retro "Atomic Age" theme with self-hosted
fonts and inline SVG icons (no CDN or JS at runtime).

A header strip shows **live weather** (current conditions, a +3h outlook, and
the next two days) from [Open-Meteo](https://open-meteo.com/) plus a clock and
Dutch date, above two widgets:

- **🚌 Bus** — real-time departures for one or more configured stops, with a
  "when to leave" countdown that subtracts your walking time. Data comes from
  [OVapi](http://v0.ovapi.nl/) — the same KV78turbo feed that drives the physical
  departure boards at the stop.
- **🗑️ Afval** — the household waste collection schedule for your address, as a
  4-week month calendar plus a "next collection per bin" summary. Data comes from
  the [Opzet afvalkalender](https://afvalkalender.purmerend.nl) used by Purmerend
  (the full-year `kalender/{year}` endpoint joined with the fraction names).

Both widgets read from an in-memory cache refreshed by background pollers, so a
slow or failed upstream never blanks the screen — it shows the last good data
with a "stale since" timestamp.

## Run locally

```bash
go run ./cmd/server
# open http://localhost:8080
```

## Run with Docker

```bash
cp .env.example .env   # then edit .env with your stop / address / coordinates
docker compose up --build
# open http://localhost:8080
```

`.env` is gitignored; `docker-compose.yml` reads it via `${VAR:-default}`
substitution and falls back to neutral examples if it's absent.

### Prebuilt image

CI publishes an image to GHCR on every push to `main`, so on a home server you
can skip the build:

```bash
docker run -d --restart unless-stopped -p 8080:8080 --env-file .env \
  ghcr.io/jimmson/thuisbord:latest
```

Point the tablet's browser at `http://<host>:8080` in fullscreen/kiosk mode.

## Configuration

All settings are environment variables (see `docker-compose.yml` for defaults):

| Variable | Default | Meaning |
|----------|---------|---------|
| `DASH_BUS_TPC` | `37400110` | OVapi TimingPointCodes, comma-separated (Ged. Singelgracht — line 305 → Amsterdam Centraal). Add more codes to show multiple stops. |
| `DASH_WALK_OFFSET_MIN` | `8` | Minutes to leave before departure (walk + buffer to arrive early) |
| `DASH_MAX_DEPARTURES` | `6` | How many bus departures to show |
| `DASH_POSTCODE` | `1011AB` | Address postcode (uppercase, no space) |
| `DASH_HOUSE_NUMBER` | `1` | Address house number |
| `DASH_LAT` / `DASH_LON` | `52.3731` / `4.8922` | Coordinates for the weather forecast |
| `DASH_WEATHER_POLL_MIN` | `15` | How often (minutes) to refresh the weather |
| `DASH_REFRESH_SECONDS` | `60` | Browser full-page auto-reload cadence |
| `DASH_BUS_POLL_SECONDS` | `30` | How often to refresh bus departures |
| `DASH_TRASH_POLL_HOURS` | `6` | How often to refresh the waste schedule |
| `DASH_TRASH_WEEKS` | `4` | Weeks shown in the afval calendar (current week + next 3) |
| `DASH_ADDR` | `:8080` | Listen address |
| `TZ` | `Europe/Amsterdam` | Timezone (embedded in the binary via `time/tzdata`) |

### Finding your bus stop code

Browse `http://v0.ovapi.nl/line/` to find your line, then
`http://v0.ovapi.nl/line/{lineid}` to read the stop list with `TimingPointCode`s,
or inspect a stop directly with `curl http://v0.ovapi.nl/tpc/{code}`.
(Use plain `http://` — the TLS cert is only valid for the apex domain.)

## Layout

```
cmd/server/main.go        wiring: config, clients, pollers, HTTP server
internal/config           env-var configuration
internal/cache            generic RWMutex cache + background poller
internal/ovapi            OVapi bus-departure client
internal/opzet            Opzet waste-schedule client
internal/weather          Open-Meteo weather client (WMO code → icon/text)
internal/web              server, view-building, templates, CSS
internal/web/icons        Lucide SVGs, embedded + inlined via the `icon` template func
internal/web/static/fonts self-hosted woff2 (Bricolage Grotesque, Jost)
```

Add a widget by writing a poller + a `{{define}}` partial and one line in
`layout.html`.

## License

[MIT](LICENSE)

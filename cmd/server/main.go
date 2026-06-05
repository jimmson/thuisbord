// Command server runs Thuisbord: a small server-rendered home dashboard showing
// real-time bus departures and the household waste collection schedule.
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "time/tzdata" // embed the IANA tz database so Europe/Amsterdam works on scratch/distroless

	"thuisbord/internal/cache"
	"thuisbord/internal/config"
	"thuisbord/internal/opzet"
	"thuisbord/internal/ovapi"
	"thuisbord/internal/weather"
	"thuisbord/internal/web"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Caches hold the last good value; pollers refresh them in the background.
	busCache := cache.New[ovapi.Board]()
	trashCache := cache.New[[]opzet.Pickup]()
	weatherCache := cache.New[weather.Report]()

	busClient := ovapi.New(cfg.Location)
	trashClient := opzet.New(cfg.Location, cfg.Postcode, cfg.HouseNumber)
	weatherClient := weather.New(cfg.Location, cfg.Lat, cfg.Lon)

	go cache.Poll(ctx, busCache, "bus", cfg.BusPoll, func(c context.Context) (ovapi.Board, error) {
		return busClient.Fetch(c, cfg.BusStopCodes)
	})
	go cache.Poll(ctx, trashCache, "trash", cfg.TrashPoll, func(c context.Context) ([]opzet.Pickup, error) {
		return trashClient.Fetch(c)
	})
	go cache.Poll(ctx, weatherCache, "weather", cfg.WeatherPoll, func(c context.Context) (weather.Report, error) {
		return weatherClient.Fetch(c)
	})

	srv, err := web.NewServer(cfg, busCache, trashCache, weatherCache)
	if err != nil {
		log.Fatalf("server: %v", err)
	}

	httpSrv := &http.Server{
		Addr:         cfg.Addr,
		Handler:      srv.Handler(),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("Thuisbord listening on %s (stops=%v, %s-%s)", cfg.Addr, cfg.BusStopCodes, cfg.Postcode, cfg.HouseNumber)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("listen: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("shutting down…")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown: %v", err)
	}
}

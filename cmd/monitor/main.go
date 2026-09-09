package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"hostpulse/internal/config"
	"hostpulse/internal/monitor"
	"hostpulse/internal/netload"
	"hostpulse/internal/state"
	"hostpulse/internal/telegram"
)

func main() {
	envPath := ".env"
	if len(os.Args) > 1 {
		envPath = os.Args[1]
	}
	cfg, err := config.Load(envPath)
	if err != nil {
		log.Fatal(err)
	}
	store, err := state.Load(cfg.StateFile)
	if err != nil {
		log.Fatal(err)
	}
	tg := telegram.New(cfg.BotToken, cfg.ChatID)
	net := netload.New(cfg.NetIface)
	app := monitor.New(cfg, tg, store, net)

	log.Printf("monitor started: check=%s report=%s", cfg.CheckInterval, cfg.ReportInterval)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	app.Run(ctx)
}

// Command udsrelay runs the UDS relay server that consoles (and udsfake) dial over UDP.
//
//	udsrelay [-listen :28300] [-timeout 5s] [-keepalive 1s] [-stats 1m] [-v]
package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"udsrelay/internal/relay"
)

func main() {
	listen := flag.String("listen", ":28300", "UDP address to listen on")
	timeout := flag.Duration("timeout", 5*time.Second, "drop a console silent this long and tell its peers it left")
	keepalive := flag.Duration("keepalive", time.Second, "keepalive interval advertised to consoles")
	statsEvery := flag.Duration("stats", time.Minute, "log counters this often (0 disables)")
	verbose := flag.Bool("v", false, "debug logging")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: udsrelay [flags]\n\nRelays 3DS local-wireless (UDS) traffic between consoles over UDP.\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	level := slog.LevelInfo
	if *verbose {
		level = slog.LevelDebug
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
	srv, err := relay.Listen(*listen, relay.Config{Timeout: *timeout, KeepaliveInterval: *keepalive, Logger: log})
	if err != nil {
		log.Error("listen", "err", err)
		os.Exit(1)
	}
	log.Info("relay listening", "addr", srv.Addr(), "timeout", *timeout, "keepalive", *keepalive)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sig
		log.Info("shutting down", "stats", fmt.Sprintf("%+v", srv.Stats()))
		srv.Close()
	}()
	if *statsEvery > 0 {
		go func() {
			for range time.Tick(*statsEvery) {
				log.Info("stats", "stats", fmt.Sprintf("%+v", srv.Stats()))
			}
		}()
	}
	if err := srv.Serve(); err != nil {
		log.Error("serve", "err", err)
		os.Exit(1)
	}
}

// Command udsrelay runs the UDS relay server that consoles (and udsfake) dial over UDP.
//
//	udsrelay -lobbies lobbies.txt [-listen :28300] [-timeout 5s] [-keepalive 1s] [-stats 1m] [-v]
//	udsrelay -genkey
//
// The lobbies file has one lobby per line, "name key", with # comments. Every console in a lobby
// uses the same key (in its config.ini); every datagram carries an HMAC keyed by it.
package main

import (
	"bufio"
	"crypto/rand"
	"encoding/base64"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
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
	lobbiesPath := flag.String("lobbies", "", "file of lobbies, one \"name key\" per line")
	var lobbies []relay.Lobby
	flag.Func("lobby", "name=key; repeatable (visible in ps: prefer -lobbies outside local tests)", func(v string) error {
		name, key, ok := strings.Cut(v, "=")
		if !ok || key == "" {
			return fmt.Errorf("want name=key, got %q", v)
		}
		lobbies = append(lobbies, relay.Lobby{Name: name, Key: key})
		return nil
	})
	genKey := flag.Bool("genkey", false, "print a new random lobby key and exit")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: udsrelay [flags]\n\nRelays 3DS local-wireless (UDS) traffic between consoles over UDP.\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if *genKey {
		b := make([]byte, 24)
		rand.Read(b)
		fmt.Println(base64.RawURLEncoding.EncodeToString(b))
		return
	}
	if *lobbiesPath != "" {
		l, err := readLobbies(*lobbiesPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, "udsrelay:", err)
			os.Exit(1)
		}
		lobbies = append(lobbies, l...)
	}
	if len(lobbies) == 0 {
		fmt.Fprintln(os.Stderr, "udsrelay: no lobbies; give -lobbies file or -lobby name=key (udsrelay -genkey makes a key)")
		os.Exit(2)
	}

	level := slog.LevelInfo
	if *verbose {
		level = slog.LevelDebug
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
	srv, err := relay.Listen(*listen, relay.Config{Timeout: *timeout, KeepaliveInterval: *keepalive, Lobbies: lobbies, Logger: log})
	if err != nil {
		log.Error("listen", "err", err)
		os.Exit(1)
	}
	log.Info("relay listening", "addr", srv.Addr(), "lobbies", len(lobbies), "timeout", *timeout, "keepalive", *keepalive)

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

// readLobbies parses "name key" lines; blank lines and # comments are skipped.
func readLobbies(path string) ([]relay.Lobby, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []relay.Lobby
	sc := bufio.NewScanner(f)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, key, ok := strings.Cut(line, " ")
		key = strings.TrimSpace(key)
		if !ok || key == "" || len(key) > relay.MaxLobbyKey {
			return nil, fmt.Errorf("%s:%d: want \"name key\" with a key of 1 to %d bytes", path, n, relay.MaxLobbyKey)
		}
		out = append(out, relay.Lobby{Name: name, Key: key})
	}
	return out, sc.Err()
}

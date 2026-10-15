package udsfake

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"udsrelay/internal/relay"
)

// TestReplayTraces replays every two-console trace in recon/traces through the relay: each
// console's recorded nwm::UDS calls run against a fake console, every reply must match the
// recording (with Azahar's quirks mapped to hardware's), and every frame either side pulls must
// be one the other side sent, byte for byte.
func TestReplayTraces(t *testing.T) {
	traces := map[string][2]string{
		"trade-01":     {"host.log", "client.log"},
		"battle-01":    {"host.log", "client.log"},
		"xy-01":        {"host.log", "client.log"},
		"drop-01":      {"host.log", "client.log"},
		"drop-02":      {"host.log", "client.log"},
		"battle-hw-01": {"console-host.log", "console-client.log"},
	}
	for name, files := range traces {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			dir := filepath.Join("..", "..", "recon", "traces", name)
			if _, err := os.Stat(dir); err != nil {
				t.Skipf("trace not present: %v", err)
			}
			srv := startRelay(t, relay.Config{}, name)
			rep, err := Replay(ReplayConfig{
				Relay: srv.Addr().String(),
				Lobby: name,
				Logs: []TraceLog{
					{Label: "host", Path: filepath.Join(dir, files[0])},
					{Label: "client", Path: filepath.Join(dir, files[1])},
				},
				Wait: 3 * time.Second,
			})
			if err != nil {
				t.Fatal(err)
			}
			var out strings.Builder
			rep.WriteText(&out)
			t.Log("\n" + out.String())
			if !rep.OK() {
				t.Fatal("replay differs from the recording")
			}
			sent, verified := 0, 0
			for _, in := range rep.Instances {
				sent += in.Sent
				verified += in.Verified
			}
			if sent == 0 || verified == 0 {
				t.Fatalf("no data exchanged: sent %d, verified %d", sent, verified)
			}
		})
	}
}

// TestReplayWithLatency replays the hardware battle with 40–60 ms of extra latency on each
// console's link, so frames can arrive out of order and events race the data.
func TestReplayWithLatency(t *testing.T) {
	if testing.Short() {
		t.Skip("slow")
	}
	dir := filepath.Join("..", "..", "recon", "traces", "battle-hw-01")
	if _, err := os.Stat(dir); err != nil {
		t.Skipf("trace not present: %v", err)
	}
	srv := startRelay(t, relay.Config{}, "latency")
	rep, err := Replay(ReplayConfig{
		Relay:  srv.Addr().String(),
		Lobby:  "latency",
		Logs:   []TraceLog{{Label: "host", Path: filepath.Join(dir, "console-host.log")}, {Label: "client", Path: filepath.Join(dir, "console-client.log")}},
		Impair: &relay.Impairment{Delay: 40 * time.Millisecond, Jitter: 20 * time.Millisecond, Seed: 3},
	})
	if err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	rep.WriteText(&out)
	t.Log("\n" + out.String())
	if !rep.OK() {
		t.Fatal("replay differs from the recording")
	}
}

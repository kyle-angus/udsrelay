package udsfake

import (
	"os"
	"os/exec"
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

// TestReplayTracesShim replays every two-console trace through the shim core's C nwm::UDS
// emulator (shim/core/tests/uds_replay.c) instead of the Go fake console: each console is a C
// process that gets the IPC requests the game sends, with the relay on its own thread as on the
// console. On top of TestReplayTraces' checks, the game's side here parses the synthesised
// beacons, joins through DecryptBeaconData's node list, and checks reply layouts, the node list
// in every status, and that every status change was signalled on the status event.
func TestReplayTracesShim(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs a C program")
	}
	bin := buildShimReplay(t)
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
				Wait: 5 * time.Second,
				Shim: bin,
			})
			if err != nil {
				t.Fatal(err)
			}
			var out strings.Builder
			rep.WriteText(&out)
			t.Log("\n" + out.String())
			if !rep.OK() {
				t.Fatal("replay through the C emulator differs from the recording")
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

func buildShimReplay(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("make"); err != nil {
		t.Skip("make not available")
	}
	core := filepath.Join("..", "..", "shim", "core")
	if out, err := exec.Command("make", "-C", core, "build/uds_replay").CombinedOutput(); err != nil {
		t.Fatalf("cannot build uds_replay: %v\n%s", err, out)
	}
	bin, err := filepath.Abs(filepath.Join(core, "build", "uds_replay"))
	if err != nil {
		t.Fatal(err)
	}
	return bin
}

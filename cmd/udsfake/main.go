// Command udsfake runs scripted fake consoles against a UDS relay, for testing without hardware.
//
//	udsfake host  [-relay addr] [-lobby key] [-packets 20] [-rate 3.5]
//	udsfake join  [-relay addr] [-lobby key] [-packets 20] [-rate 3.5]
//	udsfake replay [-relay addr] [-lobby key] [-only label] [-pace 0] host=host.log client=client.log
//
// host and join play Pokémon X/Y's trade flow: the host opens an ID8 0x01 network for up to four
// nodes on comm ID 0x00055d10, the joiner scans for it and joins, both exchange frames at X/Y's
// rate and sizes on channel 243 and check them byte for byte, then the joiner leaves and the host
// reports the leave. replay drives fake consoles from a recorded trace (see internal/udsfake).
package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"os"
	"strings"
	"sync"
	"time"

	"udsrelay/internal/relay"
	"udsrelay/internal/udsfake"
)

const (
	xyCommID  = 0x00055d10
	tradeID8  = 0x01
	xyChannel = 243
	xyFlags   = 0x02
	bindID    = 2
)

var xyPassphrase = []byte("GNgBwMcVotYs\x00")

func usage() {
	fmt.Fprint(os.Stderr, `usage: udsfake host|join|replay [flags]

host     open a trade network (ID8 0x01, 4 nodes), wait for a joiner, exchange frames,
         report the joiner's leave
join     scan for the host's network, join, exchange frames, leave
replay   replay a recorded trace's two consoles: udsfake replay host=a.log client=b.log
         Use -only host on one machine and -only client on another to split them.

Run udsfake <mode> -h for flags.
`)
	os.Exit(2)
}

type common struct {
	relay, lobby string
	loss         float64
	delay        time.Duration
	verbose      bool
}

func (c *common) register(fs *flag.FlagSet) {
	fs.StringVar(&c.relay, "relay", "127.0.0.1:28300", "relay address")
	fs.StringVar(&c.lobby, "lobby", "test-key", "lobby key, as in the relay's lobbies file; it authenticates every datagram and is never sent")
	fs.Float64Var(&c.loss, "loss", 0, "drop this fraction of datagrams each way (0..1)")
	fs.DurationVar(&c.delay, "delay", 0, "add this much latency to every datagram sent")
	fs.BoolVar(&c.verbose, "v", false, "log relay client activity")
}

func (c *common) logger() *slog.Logger {
	level := slog.LevelWarn
	if c.verbose {
		level = slog.LevelDebug
	}
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
}

func (c *common) impair() *relay.Impairment {
	if c.loss == 0 && c.delay == 0 {
		return nil
	}
	return &relay.Impairment{Loss: c.loss, Delay: c.delay, Jitter: c.delay / 4}
}

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	switch os.Args[1] {
	case "host", "join":
		os.Exit(play(os.Args[1], os.Args[2:]))
	case "replay":
		os.Exit(replay(os.Args[2:]))
	default:
		usage()
	}
}

func play(mode string, args []string) int {
	fs := flag.NewFlagSet(mode, flag.ExitOnError)
	var cm common
	cm.register(fs)
	name := fs.String("name", mode, "username shown to the peer")
	packets := fs.Int("packets", 20, "frames to send (and expect)")
	rate := fs.Float64("rate", 3.5, "frames per second")
	wait := fs.Duration("wait", 2*time.Minute, "how long to wait for the peer")
	fs.Parse(args)

	con, err := udsfake.New(udsfake.Config{Relay: cm.relay, Lobby: cm.lobby, Impair: cm.impair(), Logger: cm.logger()})
	if err != nil {
		fmt.Fprintln(os.Stderr, "relay:", err)
		return 1
	}
	defer con.Close()
	con.InitializeWithVersion(udsfake.MakeNodeInfo(*name, rand.Uint64()))
	con.Bind(bindID, xyChannel, relay.BroadcastNode, 0x2e30)

	var peer uint16
	if mode == "host" {
		con.SetProbeResponseParam()
		if r := con.BeginHostingNetwork(udsfake.MakeNetInfo(xyCommID, tradeID8, 4, 0x8000, nil), xyPassphrase); r != udsfake.ResultOK {
			fmt.Fprintf(os.Stderr, "BeginHostingNetwork failed: %#08x\n", r)
			return 1
		}
		fmt.Printf("hosting network %#08x in lobby %q; waiting for a joiner\n", con.HostedNetworkIDs()[0], cm.lobby)
		if !con.WaitStatus(func(st udsfake.ConnectionStatus) bool { return st.TotalNodes >= 2 }, *wait) {
			fmt.Fprintln(os.Stderr, "nobody joined")
			return 1
		}
		st := con.GetConnectionStatus()
		for n := uint16(2); n <= 16; n++ {
			if st.NodeBitmask&(1<<(n-1)) != 0 {
				peer = n
				break
			}
		}
		info, _ := con.NodeInfo(peer)
		fmt.Printf("node %d (%s) joined: bitmask %#04x, changed %#04x\n", peer, udsfake.NodeUsername(info), st.NodeBitmask, st.ChangedNodes)
	} else {
		deadline := time.Now().Add(*wait)
		var nets []relay.Network
		for {
			nets, _ = con.RecvBeaconBroadcastData(xyCommID, tradeID8)
			if len(nets) > 0 || time.Now().After(deadline) {
				break
			}
			time.Sleep(300 * time.Millisecond)
		}
		if len(nets) == 0 {
			fmt.Fprintln(os.Stderr, "no network found")
			return 1
		}
		n := nets[0]
		nodes, _ := con.DecryptBeaconData(n.NetworkID())
		host := ""
		if len(nodes) > 0 {
			host = udsfake.NodeUsername(nodes[0])
		}
		fmt.Printf("found network %#08x hosted by %s (%d/%d nodes)\n", n.NetworkID(), host, n.TotalNodes(), n.MaxNodes())
		if r := con.ConnectToNetwork(n.NetInfo[:], xyPassphrase, relay.ConnClient); r != udsfake.ResultOK {
			fmt.Fprintf(os.Stderr, "ConnectToNetwork failed: %#08x\n", r)
			return 1
		}
		st := con.GetConnectionStatus()
		fmt.Printf("joined as node %d: bitmask %#04x\n", st.NodeID, st.NodeBitmask)
		peer = 1
	}
	self := con.PeekStatus().NodeID

	// Exchange frames: send at the game's rate while pulling the peer's.
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		tick := time.NewTicker(time.Duration(float64(time.Second) / *rate))
		defer tick.Stop()
		for seq := 0; seq < *packets; seq++ {
			con.SendTo(peer, xyChannel, xyFlags, frame(self, seq))
			<-tick.C
		}
	}()
	got, bad := 0, 0
	idle := 5*time.Second + time.Duration(float64(time.Second) / *rate)
	for got < *packets {
		p, ok, r := con.WaitPacket(bindID, 0x5dc, idle)
		if !ok {
			if r != udsfake.ResultOK {
				fmt.Printf("network gone (%#08x)\n", r)
			}
			break
		}
		if !checkFrame(p) {
			bad++
		}
		got++
	}
	wg.Wait()
	fmt.Printf("sent %d, received %d (%d corrupt), rtt %v\n", *packets, got, bad, con.Client().RTT().Round(time.Microsecond))

	if mode == "host" {
		start := time.Now()
		if !con.WaitStatus(func(st udsfake.ConnectionStatus) bool { return st.NodeBitmask&(1<<(peer-1)) == 0 }, *wait) {
			fmt.Fprintln(os.Stderr, "joiner never left")
			return 1
		}
		st := con.GetConnectionStatus()
		fmt.Printf("node %d left after %v: bitmask %#04x, changed %#04x\n", peer, time.Since(start).Round(time.Millisecond), st.NodeBitmask, st.ChangedNodes)
		con.DestroyNetwork()
	} else {
		con.DisconnectNetwork()
		fmt.Println("left the network")
	}
	if bad > 0 || got < *packets {
		return 1
	}
	return 0
}

// frame builds a self-checking frame at one of X/Y's sizes: sender node, sequence, then a fill
// the receiver can recompute.
func frame(src uint16, seq int) []byte {
	sizes := []int{46, 48, 65, 69, 80, 69, 65, 1349}
	b := make([]byte, sizes[seq%len(sizes)])
	binary.BigEndian.PutUint16(b, src)
	binary.BigEndian.PutUint32(b[2:], uint32(seq))
	for i := 6; i < len(b); i++ {
		b[i] = byte(i*7 + seq*131 + int(src))
	}
	return b
}

func checkFrame(p udsfake.Packet) bool {
	if len(p.Data) < 6 || binary.BigEndian.Uint16(p.Data) != p.Src {
		return false
	}
	want := frame(p.Src, int(binary.BigEndian.Uint32(p.Data[2:])))
	return string(want) == string(p.Data)
}

func replay(args []string) int {
	fs := flag.NewFlagSet("replay", flag.ExitOnError)
	var cm common
	cm.register(fs)
	only := fs.String("only", "", "comma-separated labels to run here (default: all)")
	pace := fs.Float64("pace", 0, "replay at recorded speed times this (0: as fast as the peer allows)")
	wait := fs.Duration("wait", 10*time.Second, "how long a step waits for the peer")
	fs.Parse(args)
	var logs []udsfake.TraceLog
	for _, a := range fs.Args() {
		label, path, ok := strings.Cut(a, "=")
		if !ok {
			fmt.Fprintf(os.Stderr, "want label=path, got %q\n", a)
			return 2
		}
		logs = append(logs, udsfake.TraceLog{Label: label, Path: path})
	}
	cfg := udsfake.ReplayConfig{Relay: cm.relay, Lobby: cm.lobby, Logs: logs, Pace: *pace, Wait: *wait,
		Impair: cm.impair(), Logger: cm.logger()}
	if *only != "" {
		cfg.Only = strings.Split(*only, ",")
	}
	rep, err := udsfake.Replay(cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	rep.WriteText(os.Stdout)
	if !rep.OK() {
		return 1
	}
	return 0
}

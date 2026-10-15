package udsfake

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"udsrelay/internal/relay"
	"udsrelay/internal/udstrace"
)

// TraceLog names one instance's log in a recorded trace.
type TraceLog struct {
	Label string
	Path  string
}

// ReplayConfig drives Replay.
type ReplayConfig struct {
	Relay string
	Lobby string
	// Logs are the two instance logs of one trace (an Azahar host.log/client.log pair or the
	// console logs of battle-hw-01).
	Logs []TraceLog
	// Only lists the labels to run in this process; empty runs all. Running one side here and
	// the other in a second process replays a trace across two machines.
	Only []string
	// Wait bounds how long a step waits for the peer (a network to scan, a packet, a status
	// change). Default 5 s.
	Wait time.Duration
	// Pace replays at recorded speed times Pace; 0 replays as fast as the peer allows.
	Pace float64
	// Shim, when set, is the path to shim/core's uds_replay: each console is then the shim
	// core's C emulator, driven through IPC requests, instead of the Go fake console.
	Shim   string
	Impair *relay.Impairment
	Client relay.Options
	Logger *slog.Logger
}

// Report is the outcome of a replay.
type Report struct {
	Instances []*InstanceReport
}

// OK reports whether every instance replayed cleanly.
func (r *Report) OK() bool {
	for _, in := range r.Instances {
		if !in.OK() {
			return false
		}
	}
	return true
}

// InstanceReport is one console's replay.
type InstanceReport struct {
	Label    string
	Hardware bool // the log came from the console shim, not Azahar
	Calls    int
	Skipped  map[string]int // commands the fake does not model
	// Mismatches lists replies that differ from the recording, after allowing for the known
	// Azahar/hardware differences.
	Mismatches []Mismatch
	Sent       int
	// Pulled counts frames taken from bind queues; Verified those that matched a frame the
	// peer sent, byte for byte.
	Pulled, Verified int
	// Lost counts peer frames never pulled that the peer sent before the last one pulled.
	Lost         int
	WaitTimeouts int
	Vanished     bool // the log ends with a network up, so the console dropped off unannounced
	Duration     time.Duration
	Err          error
}

// OK reports a clean replay.
func (r *InstanceReport) OK() bool {
	return r.Err == nil && len(r.Mismatches) == 0 && r.WaitTimeouts == 0 && r.Pulled == r.Verified
}

// Mismatch is one reply that differs from the recording.
type Mismatch struct {
	Index  int
	Offset time.Duration // since the instance's first call
	Call   string
	Want   string
	Got    string
}

func (m Mismatch) String() string {
	return fmt.Sprintf("#%d +%.3fs %s: want %s, got %s", m.Index, m.Offset.Seconds(), m.Call, m.Want, m.Got)
}

// WriteText prints a human-readable report.
func (r *Report) WriteText(w io.Writer) {
	for _, in := range r.Instances {
		kind := "Azahar"
		if in.Hardware {
			kind = "hardware"
		}
		status := "ok"
		if !in.OK() {
			status = "FAILED"
		}
		fmt.Fprintf(w, "%s (%s log): %s in %v\n", in.Label, kind, status, in.Duration.Round(time.Millisecond))
		fmt.Fprintf(w, "  calls %d, sent %d, pulled %d, verified byte for byte %d, peer frames skipped %d\n",
			in.Calls, in.Sent, in.Pulled, in.Verified, in.Lost)
		if in.Vanished {
			fmt.Fprintf(w, "  log ends with a network up: the console dropped off without leaving\n")
		}
		if len(in.Skipped) > 0 {
			var keys []string
			for k := range in.Skipped {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			parts := make([]string, len(keys))
			for i, k := range keys {
				parts[i] = fmt.Sprintf("%s x%d", k, in.Skipped[k])
			}
			fmt.Fprintf(w, "  not modelled: %s\n", strings.Join(parts, ", "))
		}
		if in.WaitTimeouts > 0 {
			fmt.Fprintf(w, "  waits timed out: %d\n", in.WaitTimeouts)
		}
		for i, m := range in.Mismatches {
			if i == 20 {
				fmt.Fprintf(w, "  ... %d more mismatches\n", len(in.Mismatches)-i)
				break
			}
			fmt.Fprintf(w, "  mismatch %s\n", m)
		}
		if in.Err != nil {
			fmt.Fprintf(w, "  error: %v\n", in.Err)
		}
	}
}

type script struct {
	label    string
	hardware bool
	calls    []*udstrace.Call
	sends    [][]byte // payloads of every SendTo, in order
	// pos is the number of calls this instance has finished replaying; the peer reads it.
	pos     atomic.Int64
	running bool // replayed in this process, so pos moves
}

// isTeardown reports whether a recorded call left a network: Shutdown, or a DestroyNetwork or
// DisconnectNetwork that succeeded (X/Y also calls DestroyNetwork as a client, and it fails).
func isTeardown(c *udstrace.Call) bool {
	switch c.Cmd {
	case udstrace.CmdShutdown:
		return true
	case udstrace.CmdDestroyNetwork, udstrace.CmdDisconnectNetwork:
		r, ok := c.Result()
		return !ok || r == 0
	}
	return false
}

// leaves reports whether replaying c now takes the console out of a network that has peers.
func leaves(c *udstrace.Call, st ConnectionStatus) bool {
	if st.TotalNodes < 2 {
		return false
	}
	switch c.Cmd {
	case udstrace.CmdShutdown:
		return st.Status != StatusNotConnected
	case udstrace.CmdDestroyNetwork:
		return st.Status == StatusConnectedAsHost
	case udstrace.CmdDisconnectNetwork:
		return st.Status == StatusConnectedAsClient || st.Status == StatusConnectedAsSpectator
	}
	return false
}

func isDataPull(c *udstrace.Call) bool {
	r, ok := c.Result()
	return c.Cmd == udstrace.CmdPullPacket && ok && r == 0 && word(c.Reply, 2) > 0
}

// awaitPeerPulls holds back a teardown until the peer has pulled every frame it recorded
// pulling before its own next teardown. The recording has the two sides' timing; the replay
// does not, so without this a host that runs ahead could close the network under a client
// that still has frames to read (drop-02: the client vanished, and the host only tore down 23 s
// later).
func awaitPeerPulls(peer *script, wait time.Duration) {
	if !peer.running {
		return
	}
	deadline := time.Now().Add(wait)
	p := int(peer.pos.Load())
	last := -1
	for j := p; j < len(peer.calls) && !isTeardown(peer.calls[j]); j++ {
		if isDataPull(peer.calls[j]) {
			last = j
		}
	}
	for last >= 0 && int(peer.pos.Load()) <= last && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
}

func loadScript(tl TraceLog) (*script, error) {
	f, err := os.Open(tl.Path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	recs, err := udstrace.Parse(f, tl.Label)
	if err != nil {
		return nil, err
	}
	s := &script{label: tl.Label}
	for _, it := range udstrace.Build(recs) {
		if it.Call == nil || it.Call.Port != udstrace.PortUDS {
			continue
		}
		if it.Call.Req.Sess != "" {
			s.hardware = true
		}
		s.calls = append(s.calls, it.Call)
		if it.Call.Cmd == udstrace.CmdSendTo {
			s.sends = append(s.sends, sendPayload(it.Call))
		}
	}
	return s, nil
}

// sendPayload is the recorded frame, padded to its recorded size where the log truncated it.
func sendPayload(c *udstrace.Call) []byte {
	size := int(word(c.Req.Words, 5))
	b := c.Blob("send")
	var data []byte
	if b != nil {
		data, size = b.Data, b.Size
	}
	out := make([]byte, size)
	n := copy(out, data)
	for i := n; i < size; i++ {
		out[i] = byte(i*31 + 7)
	}
	return out
}

func word(ws []uint32, i int) uint32 {
	if i < len(ws) {
		return ws[i]
	}
	return 0
}

// Replay runs the trace's instances against the relay as fake consoles, one goroutine each.
// Each instance issues its recorded nwm::UDS calls in order and compares replies with the
// recording. Steps that depend on the peer wait for it: a scan before a join retries until the
// network shows up, a recorded packet waits for a frame, and a recorded status waits for the
// node bitmask it shows. Every pulled frame must equal, byte for byte, a frame the peer sent.
func Replay(cfg ReplayConfig) (*Report, error) {
	if len(cfg.Logs) != 2 {
		return nil, fmt.Errorf("replay needs the two logs of one trace, got %d", len(cfg.Logs))
	}
	if cfg.Wait == 0 {
		cfg.Wait = 5 * time.Second
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.New(slog.NewTextHandler(discardWriter{}, nil))
	}
	scripts := make([]*script, len(cfg.Logs))
	for i, tl := range cfg.Logs {
		s, err := loadScript(tl)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", tl.Label, err)
		}
		scripts[i] = s
	}
	run := map[string]bool{}
	for _, l := range cfg.Only {
		run[l] = true
	}
	for _, s := range scripts {
		s.running = len(run) == 0 || run[s.label]
	}
	rep := &Report{}
	var wg sync.WaitGroup
	for i, s := range scripts {
		if !s.running {
			continue
		}
		in := &InstanceReport{Label: s.label, Hardware: s.hardware, Skipped: map[string]int{}}
		rep.Instances = append(rep.Instances, in)
		peer := scripts[1-i]
		wg.Add(1)
		go func() {
			defer wg.Done()
			start := time.Now()
			replayOne(cfg, s, peer, in)
			in.Duration = time.Since(start)
		}()
	}
	wg.Wait()
	return rep, nil
}

type matcher struct {
	sent    [][]byte
	matched []bool
	first   int // lowest unmatched index
	last    int // highest matched index, -1 before the first match
}

// match finds the earliest unmatched peer frame equal to p, allowing for loss and reordering.
func (m *matcher) match(p []byte) bool {
	const window = 256
	for j := m.first; j < len(m.sent) && j < m.first+window; j++ {
		if !m.matched[j] && bytes.Equal(m.sent[j], p) {
			m.matched[j] = true
			m.last = max(m.last, j)
			for m.first < len(m.sent) && m.matched[m.first] {
				m.first++
			}
			return true
		}
	}
	return false
}

// skipped counts peer frames never pulled that were sent before the last one pulled.
func (m *matcher) skipped() int {
	n := 0
	for j := 0; j < m.last; j++ {
		if !m.matched[j] {
			n++
		}
	}
	return n
}

// ReplayConsole is what a replay drives: the Go fake console (Console) or the shim core's C
// emulator (ShimConsole).
type ReplayConsole interface {
	InitializeWithVersion(self [relay.NodeInfoSize]byte) uint32
	Shutdown() uint32
	SetProbeResponseParam() uint32
	Cmd0023() (uint32, uint32)
	BeginHostingNetwork(netInfo, passphrase []byte) uint32
	DestroyNetwork() uint32
	SetApplicationData(data []byte) uint32
	GetApplicationData() ([]byte, uint32)
	GetChannel() (uint32, uint8)
	RecvBeaconBroadcastData(commID uint32, id8 uint8) ([]relay.Network, uint32)
	LastScan() []relay.Network
	DecryptBeaconData(networkID uint32) ([][relay.NodeInfoSize]byte, uint32)
	ConnectToNetwork(netInfo, passphrase []byte, connType uint8) uint32
	DisconnectNetwork() uint32
	EjectClient(node uint16) uint32
	GetConnectionStatus() ConnectionStatus
	PeekStatus() ConnectionStatus
	WaitStatus(ok func(ConnectionStatus) bool, timeout time.Duration) bool
	Bind(bindID uint32, channel uint8, node uint16, recvBufSize int) uint32
	Unbind(bindID uint32) uint32
	SendTo(dst uint16, channel, flags uint8, data []byte) uint32
	PullPacket(bindID uint32, maxSize int) (Packet, bool, uint32)
	WaitPacket(bindID uint32, maxSize int, timeout time.Duration) (Packet, bool, uint32)
	PollPacket(bindID uint32) (int, uint32)
	Kill()
	Close() error
}

func newReplayConsole(cfg ReplayConfig, label string) (ReplayConsole, error) {
	if cfg.Shim != "" {
		return NewShimConsole(cfg.Shim, cfg.Relay, cfg.Lobby, label)
	}
	return New(Config{Relay: cfg.Relay, Lobby: cfg.Lobby, Impair: cfg.Impair, Client: cfg.Client,
		Logger: cfg.Logger.With("console", label)})
}

// maxWaitTimeouts ends an instance's replay early: past this many timed-out waits the two sides
// have diverged.
const maxWaitTimeouts = 3

func replayOne(cfg ReplayConfig, s, peer *script, in *InstanceReport) {
	c, err := newReplayConsole(cfg, s.label)
	if err != nil {
		in.Err = err
		return
	}
	if sc, ok := c.(*ShimConsole); ok {
		defer func() {
			for _, p := range sc.Problems() {
				in.Mismatches = append(in.Mismatches, Mismatch{Index: -1, Call: "emulator", Want: "no protocol faults", Got: p})
			}
		}()
	}
	m := &matcher{sent: peer.sends, matched: make([]bool, len(peer.sends)), last: -1}
	defer func() { in.Lost = m.skipped() }()
	start := time.Now()
	var first uint64
	if len(s.calls) > 0 {
		first = s.calls[0].Req.Wall
	}
	sendIdx := 0
	defer s.pos.Store(int64(len(s.calls)))
	for i, call := range s.calls {
		s.pos.Store(int64(i))
		if in.WaitTimeouts >= maxWaitTimeouts {
			// The peer is not doing what the recording says; every further wait would time
			// out too.
			in.Err = fmt.Errorf("gave up at call %d of %d after %d waits for the peer timed out", i, len(s.calls),
				in.WaitTimeouts)
			break
		}
		if isTeardown(call) && leaves(call, c.PeekStatus()) {
			awaitPeerPulls(peer, cfg.Wait)
		}
		offset := time.Duration(call.Req.Wall-first) * time.Microsecond
		if cfg.Pace > 0 {
			if d := time.Until(start.Add(time.Duration(float64(offset) / cfg.Pace))); d > 0 {
				time.Sleep(d)
			}
		}
		in.Calls++
		mismatch := func(want, got string) {
			in.Mismatches = append(in.Mismatches, Mismatch{Index: i, Offset: offset, Call: udstrace.Describe(call), Want: want, Got: got})
		}
		checkResult := func(got uint32) {
			if want, ok := expectedResult(call); ok && want != got {
				mismatch(udstrace.ResultString(want), udstrace.ResultString(got))
			}
		}
		w := call.Req.Words
		switch call.Cmd {
		case udstrace.CmdInitializeWithVersion:
			var self [relay.NodeInfoSize]byte
			copy(self[:], wordsToBytes(w[2:min(12, len(w))]))
			checkResult(c.InitializeWithVersion(self))
		case udstrace.CmdShutdown:
			checkResult(c.Shutdown())
		case 0x0021:
			checkResult(c.SetProbeResponseParam())
		case 0x0023:
			r, word2 := c.Cmd0023()
			checkResult(r)
			if want, ok := call.Result(); ok && want == 0 && len(call.Reply) > 2 && call.Reply[2]&0xFFFF != word2&0xFFFF {
				mismatch(fmt.Sprintf("word %#x", call.Reply[2]), fmt.Sprintf("word %#x", word2))
			}
		case udstrace.CmdBeginHostingNetwork:
			ni, pp := call.Blob("netinfo"), call.Blob("passphrase")
			if ni == nil || pp == nil {
				in.Skipped["BeginHostingNetwork without payload"]++
				continue
			}
			checkResult(c.BeginHostingNetwork(ni.Data, pp.Data))
		case udstrace.CmdDestroyNetwork:
			checkResult(c.DestroyNetwork())
		case udstrace.CmdSetApplicationData:
			b := call.Blob("appdata")
			if b == nil {
				in.Skipped["SetApplicationData without payload"]++
				continue
			}
			checkResult(c.SetApplicationData(b.Data))
		case udstrace.CmdGetApplicationData:
			_, r := c.GetApplicationData()
			checkResult(r)
		case udstrace.CmdGetChannel:
			r, ch := c.GetChannel()
			checkResult(r)
			if want, ok := call.Result(); ok && want == 0 && len(call.Reply) > 2 && call.Reply[2] != uint32(ch) {
				mismatch(fmt.Sprintf("channel %d", call.Reply[2]), fmt.Sprintf("channel %d", ch))
			}
		case udstrace.CmdRecvBeaconBroadcastData:
			commID, id8 := word(w, 15), uint8(word(w, 16))
			if wantComm, wantID8, ok := nextJoin(s.calls[i+1:]); ok {
				// The recording found the network it joins next: wait for the peer to host it.
				deadline := time.Now().Add(cfg.Wait)
				for {
					nets, r := c.RecvBeaconBroadcastData(commID, id8)
					if findNetwork(nets, wantComm, wantID8) != nil {
						checkResult(r)
						break
					}
					if time.Now().After(deadline) {
						in.WaitTimeouts++
						mismatch("the network joined next", "not found")
						break
					}
					time.Sleep(50 * time.Millisecond)
				}
			} else {
				_, r := c.RecvBeaconBroadcastData(commID, id8)
				checkResult(r)
			}
		case udstrace.CmdDecryptBeaconData:
			var id uint32
			if nets := c.LastScan(); len(nets) > 0 {
				id = nets[0].NetworkID()
			}
			_, r := c.DecryptBeaconData(id)
			checkResult(r)
		case udstrace.CmdConnectToNetwork:
			ni, pp := call.Blob("netinfo"), call.Blob("passphrase")
			if ni == nil || pp == nil || len(ni.Data) < 0x18 {
				in.Skipped["ConnectToNetwork without payload"]++
				continue
			}
			want := findNetwork(c.LastScan(), binary.BigEndian.Uint32(ni.Data[0x10:]), ni.Data[0x14])
			if want == nil {
				mismatch("a scanned network to join", "none")
				continue
			}
			connType := uint8(relay.ConnClient)
			if ct := ni.Extra["conn_type"]; ct == "2" {
				connType = relay.ConnSpectator
			}
			checkResult(c.ConnectToNetwork(want.NetInfo[:], pp.Data, connType))
		case udstrace.CmdDisconnectNetwork:
			checkResult(c.DisconnectNetwork())
		case udstrace.CmdEjectClient:
			checkResult(c.EjectClient(uint16(word(w, 1))))
		case udstrace.CmdGetConnectionStatus:
			want, ok := udstrace.ParseConnectionStatus(call.Reply)
			if !ok {
				c.GetConnectionStatus()
				continue
			}
			if !c.WaitStatus(func(st ConnectionStatus) bool {
				return st.Status == want.Status && st.NodeBitmask == want.NodeBitmask && st.TotalNodes == want.TotalNodes
			}, cfg.Wait) {
				in.WaitTimeouts++
			}
			got := c.GetConnectionStatus()
			if d := statusDiff(want, got); d != "" {
				mismatch(want.String(), d)
			}
		case udstrace.CmdBind:
			checkResult(c.Bind(word(w, 1), uint8(word(w, 3)), uint16(word(w, 4)), int(word(w, 2))))
		case udstrace.CmdUnbind:
			checkResult(c.Unbind(word(w, 1)))
		case udstrace.CmdSendTo:
			payload := s.sends[sendIdx]
			sendIdx++
			in.Sent++
			checkResult(c.SendTo(uint16(word(w, 2)), uint8(word(w, 3)), uint8(word(w, 6)), payload))
		case udstrace.CmdPullPacket:
			bindID, maxSize := word(w, 1), int(word(w, 3))
			want, haveWant := call.Result()
			var (
				p  Packet
				ok bool
				r  uint32
			)
			switch {
			case haveWant && want == 0 && word(call.Reply, 2) > 0:
				p, ok, r = c.WaitPacket(bindID, maxSize, cfg.Wait)
				if !ok && r == ResultOK {
					in.WaitTimeouts++
					mismatch(fmt.Sprintf("a %d-byte frame", word(call.Reply, 2)), "none arrived")
				}
			case haveWant && want == 0:
				// The recording found nothing here. Take nothing either, even if a frame is
				// queued: it belongs to the next pull the recording says got one, and taking
				// it here would leave this side waiting for a frame the peer has not sent.
				_, r = c.PollPacket(bindID)
			default:
				p, ok, r = c.PullPacket(bindID, maxSize)
			}
			checkResult(r)
			if ok {
				in.Pulled++
				if m.match(p.Data) {
					in.Verified++
				} else {
					mismatch("a frame the peer sent", fmt.Sprintf("%d bytes from node %d matching none", len(p.Data), p.Src))
				}
			}
		default:
			in.Skipped[call.Name]++
		}
	}
	if st := c.PeekStatus(); st.Status != StatusNotConnected {
		in.Vanished = true
		c.Kill()
		return
	}
	c.Close()
}

// expectedResult is the reply code the fake should give, allowing for the differences between
// Azahar and hardware that the fake resolves in favour of hardware.
func expectedResult(c *udstrace.Call) (uint32, bool) {
	if c.Unimplemented {
		switch c.Cmd {
		case 0x0021, 0x0023:
			return ResultOK, true // hardware: success
		}
		return 0, false
	}
	code, ok := c.Result()
	if !ok {
		return 0, false
	}
	if code == ResultAzaharNotHosting && c.Cmd == udstrace.CmdDestroyNetwork {
		return ResultInvalidState, true
	}
	return code, true
}

// statusDiff compares a recorded status with the fake's. Not-connected statuses compare only
// the status: Azahar zeroes the reason and hardware does not, and the node ID left behind
// depends on history the fake does not replay (hardware showed node 2 on a host after re-init).
func statusDiff(want udstrace.ConnectionStatus, got ConnectionStatus) string {
	if want.Status != got.Status {
		return fmt.Sprintf("status %d", got.Status)
	}
	if want.Status == StatusNotConnected {
		return ""
	}
	if want.NodeID != got.NodeID || want.NodeBitmask != got.NodeBitmask || want.ChangedNodes != got.ChangedNodes ||
		want.TotalNodes != got.TotalNodes || want.MaxNodes != got.MaxNodes || want.Reason != got.Reason {
		return fmt.Sprintf("reason=%d node=%d nodes=%d/%d bitmask=0x%04x changed=0x%04x",
			got.Reason, got.NodeID, got.TotalNodes, got.MaxNodes, got.NodeBitmask, got.ChangedNodes)
	}
	return ""
}

// nextJoin reports the network the instance joins next, if it joins before scanning again.
func nextJoin(rest []*udstrace.Call) (commID uint32, id8 uint8, ok bool) {
	for _, c := range rest {
		switch c.Cmd {
		case udstrace.CmdRecvBeaconBroadcastData:
			return 0, 0, false
		case udstrace.CmdConnectToNetwork:
			ni := c.Blob("netinfo")
			if ni == nil || len(ni.Data) < 0x18 {
				return 0, 0, false
			}
			return binary.BigEndian.Uint32(ni.Data[0x10:]), ni.Data[0x14], true
		}
	}
	return 0, 0, false
}

func findNetwork(nets []relay.Network, commID uint32, id8 uint8) *relay.Network {
	for i := range nets {
		if nets[i].CommID() == commID && nets[i].ID8() == id8 {
			return &nets[i]
		}
	}
	return nil
}

func wordsToBytes(ws []uint32) []byte {
	b := make([]byte, 4*len(ws))
	for i, w := range ws {
		binary.LittleEndian.PutUint32(b[4*i:], w)
	}
	return b
}

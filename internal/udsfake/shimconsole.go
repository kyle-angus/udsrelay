package udsfake

import (
	"bufio"
	"bytes"
	"crypto/sha1"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"udsrelay/internal/relay"
	"udsrelay/internal/udstrace"
)

// ShimConsole is one console running the shim core's C nwm::UDS emulator
// (shim/core/tests/uds_replay.c) in a child process. It plays the game: every method sends the
// IPC request the game sends, in hardware's layout (battle-hw-01), and reads the reply. A scan
// gets synthesised 802.11 beacons back, which ShimConsole parses as the game does, rebuilding
// each network struct from the beacon's tags and asking the emulator's DecryptBeaconData for the
// node list. Joins then use the rebuilt struct.
type ShimConsole struct {
	cmd    *exec.Cmd
	in     io.WriteCloser
	out    *bufio.Reader
	stderr *tailBuffer
	label  string

	mu          sync.Mutex
	scan        []scannedBeacon
	statusEvent uint32
	// For checking that every status change was signalled on the status event.
	lastStatus  ConnectionStatus
	lastSignals uint32
	problems    []string
	exited      bool
}

type scannedBeacon struct {
	network    relay.Network
	tag0, tag1 []byte
}

// NewShimConsole starts the C emulator at bin (shim/core/build/uds_replay) against the relay.
func NewShimConsole(bin, relayAddr, lobby, label string) (*ShimConsole, error) {
	host, port, err := net.SplitHostPort(relayAddr)
	if err != nil {
		return nil, err
	}
	c := &ShimConsole{label: label, stderr: &tailBuffer{max: 64 << 10}}
	c.cmd = exec.Command(bin, host, port, lobby, label)
	c.cmd.Stderr = c.stderr
	if c.in, err = c.cmd.StdinPipe(); err != nil {
		return nil, err
	}
	stdout, err := c.cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	c.out = bufio.NewReaderSize(stdout, 1<<20)
	if err := c.cmd.Start(); err != nil {
		return nil, err
	}
	c.lastStatus.Status = StatusNotConnected
	return c, nil
}

// Log is the tail of the emulator's log (its stderr).
func (c *ShimConsole) Log() string { return c.stderr.String() }

// Problems lists protocol faults seen so far: status changes that were not signalled, malformed
// replies, beacons the game could not parse, and handle misuse reported when the process exits.
func (c *ShimConsole) Problems() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.problems...)
}

func (c *ShimConsole) problem(format string, args ...any) {
	c.problems = append(c.problems, fmt.Sprintf(format, args...))
}

// call sends one line and reads the answer. Called with c.mu held.
func (c *ShimConsole) call(line string) (string, error) {
	if c.exited {
		return "", fmt.Errorf("emulator exited")
	}
	if _, err := io.WriteString(c.in, line+"\n"); err != nil {
		return "", err
	}
	resp, err := c.out.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("emulator: %v\n%s", err, c.stderr.String())
	}
	resp = strings.TrimRight(resp, "\n")
	if strings.HasPrefix(resp, "err ") {
		return "", fmt.Errorf("emulator: %s", resp)
	}
	return resp, nil
}

// ipcReply is a decoded "rsp" line.
type ipcReply struct {
	words []uint32
	bufs  [][]byte
}

func (r ipcReply) result() uint32 { return word(r.words, 1) }

// request sends an IPC request; bufs holds one payload per static buffer descriptor.
func (c *ShimConsole) request(words []uint32, bufs ...[]byte) ipcReply {
	var b strings.Builder
	b.WriteString("req ")
	for i, w := range words {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, "%x", w)
	}
	for _, p := range bufs {
		b.WriteByte(' ')
		if len(p) == 0 {
			b.WriteByte('-')
		} else {
			b.WriteString(hex.EncodeToString(p))
		}
	}
	resp, err := c.call(b.String())
	if err != nil {
		c.problem("%#08x: %v", words[0], err)
		return ipcReply{words: []uint32{words[0], ResultInvalidState}}
	}
	fields := strings.Fields(resp)
	if len(fields) < 2 || fields[0] != "rsp" {
		c.problem("%#08x: unexpected answer %.80q", words[0], resp)
		return ipcReply{words: []uint32{words[0], ResultInvalidState}}
	}
	var r ipcReply
	for _, w := range strings.Split(fields[1], ",") {
		v, _ := strconv.ParseUint(w, 16, 32)
		r.words = append(r.words, uint32(v))
	}
	for _, f := range fields[2:] {
		if f == "-" {
			r.bufs = append(r.bufs, nil)
			continue
		}
		p, _ := hex.DecodeString(f)
		r.bufs = append(r.bufs, p)
	}
	if want := uint16(words[0] >> 16); uint16(r.words[0]>>16) != want {
		c.problem("reply to %#04x has header %#08x", want, r.words[0])
	}
	return r
}

func (c *ShimConsole) simple(words ...uint32) uint32 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.request(words).result()
}

func staticDesc(size, id int) uint32 { return uint32(size)<<14 | uint32(id&0xF)<<10 | 2 }

func bytesToWords(b []byte) []uint32 {
	out := make([]uint32, (len(b)+3)/4)
	p := make([]byte, 4*len(out))
	copy(p, b)
	for i := range out {
		out[i] = binary.LittleEndian.Uint32(p[4*i:])
	}
	return out
}

// InitializeWithVersion sends the NodeInfo and keeps the status event the emulator returns.
func (c *ShimConsole) InitializeWithVersion(self [relay.NodeInfoSize]byte) uint32 {
	c.mu.Lock()
	defer c.mu.Unlock()
	words := []uint32{0x001B0302, 0x14000}
	words = append(words, bytesToWords(self[:])...)
	words = append(words, 0x400, 0, 0) // version, copy-handle descriptor, shared memory handle
	r := c.request(words)
	if r.result() == ResultOK {
		if len(r.words) < 4 || r.words[0] != 0x001B0042 || r.words[2] != 0 || r.words[3] == 0 {
			c.problem("InitializeWithVersion reply %#x carries no event", r.words)
		} else {
			c.statusEvent = r.words[3]
			c.lastSignals = c.signals(c.statusEvent)
			c.lastStatus = ConnectionStatus{Status: StatusNotConnected}
		}
	}
	return r.result()
}

func (c *ShimConsole) Shutdown() uint32 {
	c.mu.Lock()
	defer c.mu.Unlock()
	r := c.request([]uint32{0x00030000}).result()
	c.statusEvent = 0
	return r
}

func (c *ShimConsole) SetProbeResponseParam() uint32 {
	return c.simple(0x00210080, 0x00321f00, 0x00001d00)
}

func (c *ShimConsole) Cmd0023() (uint32, uint32) {
	c.mu.Lock()
	defer c.mu.Unlock()
	r := c.request([]uint32{0x00230000})
	return r.result(), word(r.words, 2)
}

func (c *ShimConsole) BeginHostingNetwork(netInfo, passphrase []byte) uint32 {
	c.mu.Lock()
	defer c.mu.Unlock()
	ni := make([]byte, relay.NetInfoSize)
	copy(ni, netInfo)
	return c.request([]uint32{0x001D0044, uint32(len(passphrase)), staticDesc(len(ni), 1), 0,
		staticDesc(len(passphrase), 0), 0}, ni, passphrase).result()
}

func (c *ShimConsole) DestroyNetwork() uint32 { return c.simple(0x00080000) }

func (c *ShimConsole) SetApplicationData(data []byte) uint32 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.request([]uint32{0x00100042, uint32(len(data)), staticDesc(len(data), 4), 0}, data).result()
}

func (c *ShimConsole) GetApplicationData() ([]byte, uint32) {
	c.mu.Lock()
	defer c.mu.Unlock()
	r := c.request([]uint32{0x00110040, relay.MaxAppData})
	if r.result() != ResultOK || len(r.bufs) == 0 {
		return nil, r.result()
	}
	return r.bufs[0], r.result()
}

func (c *ShimConsole) GetChannel() (uint32, uint8) {
	c.mu.Lock()
	defer c.mu.Unlock()
	r := c.request([]uint32{0x001A0000})
	return r.result(), uint8(word(r.words, 2))
}

const beaconBufSize = 0x4000

// RecvBeaconBroadcastData scans and parses the beacons like the game: each network struct is
// rebuilt from its beacon, and its node list comes from DecryptBeaconData.
func (c *ShimConsole) RecvBeaconBroadcastData(commID uint32, id8 uint8) ([]relay.Network, uint32) {
	c.mu.Lock()
	defer c.mu.Unlock()
	words := []uint32{0x000F0404, beaconBufSize, 0x00020001, 0x006e0421, 0xffffffff, 0x0000ffff,
		0, 0, 0, 0, 0, 0, 0, 0, 0, commID, uint32(id8),
		0, 0, // copy-handle descriptor and the scan event the game passes
		beaconBufSize<<4 | 0xC, 0} // mapped buffer, writable
	r := c.request(words)
	c.scan = nil
	if r.result() != ResultOK {
		return nil, r.result()
	}
	if len(r.words) < 4 || r.words[2] != words[19] || len(r.bufs) != 1 {
		c.problem("RecvBeaconBroadcastData reply %#x does not return the mapped buffer", r.words)
		return nil, r.result()
	}
	beacons, err := parseBeaconReply(r.bufs[0], beaconBufSize)
	if err != nil {
		c.problem("beacons: %v", err)
	}
	var nets []relay.Network
	for _, b := range beacons {
		nodes, res := c.decrypt(b)
		if res != ResultOK {
			c.problem("DecryptBeaconData during scan: %s", udstrace.ResultString(res))
			continue
		}
		for i, n := range nodes {
			if id := binary.LittleEndian.Uint16(n[0x20:]); id != 0 {
				if int(id) != i+1 {
					c.problem("DecryptBeaconData slot %d holds node %d", i, id)
				}
				b.network.Bitmask |= 1 << i
				b.network.Nodes[i] = n
			}
		}
		c.scan = append(c.scan, b)
		nets = append(nets, b.network)
	}
	return nets, r.result()
}

// parseBeaconReply reads NWM's BeaconDataReply: a 12-byte header, then entries of
// BeaconEntryHeader (0x1C bytes) plus an 802.11 beacon body.
func parseBeaconReply(buf []byte, size int) ([]scannedBeacon, error) {
	if len(buf) < 12 {
		return nil, fmt.Errorf("reply buffer is %d bytes", len(buf))
	}
	maxOut := binary.LittleEndian.Uint32(buf)
	total := int(binary.LittleEndian.Uint32(buf[4:]))
	entries := int(binary.LittleEndian.Uint32(buf[8:]))
	if int(maxOut) != size || total > len(buf) || total < 12 {
		return nil, fmt.Errorf("bad reply header: max %d, total %d", maxOut, total)
	}
	var out []scannedBeacon
	off := 12
	for i := 0; i < entries; i++ {
		if off+0x1C > total {
			return out, fmt.Errorf("entry %d runs past the reply", i)
		}
		e := buf[off:]
		esize := int(binary.LittleEndian.Uint32(e))
		hsize := int(binary.LittleEndian.Uint32(e[0x18:]))
		if hsize != 0x1C || esize < hsize || off+esize > total || binary.LittleEndian.Uint32(e[0x14:]) != uint32(esize) {
			return out, fmt.Errorf("entry %d: bad sizes %d/%d", i, esize, hsize)
		}
		b, err := parseBeacon(e[hsize:esize])
		if err != nil {
			return out, fmt.Errorf("entry %d: %v", i, err)
		}
		copy(b.network.NetInfo[0:6], e[8:14]) // host MAC
		b.network.NetInfo[6] = e[5]           // channel
		out = append(out, b)
		off += esize
	}
	if off != total {
		return out, fmt.Errorf("entries end at %d, header says %d", off, total)
	}
	return out, nil
}

var nintendoOUI = []byte{0x00, 0x1F, 0x32}

// parseBeacon rebuilds the network struct from a beacon's Nintendo tags and keeps the two data
// tags (from the OUI on) for DecryptBeaconData, checking the network-info tag's SHA-1 as NWM may.
func parseBeacon(frame []byte) (scannedBeacon, error) {
	var b scannedBeacon
	if len(frame) < 12 {
		return b, fmt.Errorf("frame of %d bytes", len(frame))
	}
	haveInfo := false
	for i := 12; i < len(frame); {
		if i+2 > len(frame) || i+2+int(frame[i+1]) > len(frame) {
			return b, fmt.Errorf("tag at %d runs past the frame", i)
		}
		id, body := frame[i], frame[i+2:i+2+int(frame[i+1])]
		i += 2 + len(body)
		if id != 0xDD || len(body) < 4 || !bytes.Equal(body[:3], nintendoOUI) {
			continue
		}
		switch body[3] {
		case 0x15:
			if len(body) < 52 || len(body) != 52+int(body[51]) {
				return b, fmt.Errorf("network-info tag of %d bytes", len(body))
			}
			check := append([]byte(nil), body...)
			clear(check[0x1F : 0x1F+20])
			if sum := sha1.Sum(check); !bytes.Equal(sum[:], body[0x1F:0x1F+20]) {
				return b, fmt.Errorf("network-info tag SHA-1 does not match")
			}
			copy(b.network.NetInfo[0x0C:0x0C+0x1F], body[:0x1F])
			b.network.NetInfo[0x3F] = body[51]
			copy(b.network.NetInfo[0x40:], body[52:])
			haveInfo = true
		case 0x18:
			b.tag0 = append([]byte(nil), body...)
		case 0x19:
			b.tag1 = append([]byte(nil), body...)
		}
	}
	if !haveInfo || b.tag0 == nil {
		return b, fmt.Errorf("beacon lacks the network-info or data tag")
	}
	return b, nil
}

// decrypt asks the emulator for a beacon's node list. Called with c.mu held.
func (c *ShimConsole) decrypt(b scannedBeacon) ([relay.MaxNodes][relay.NodeInfoSize]byte, uint32) {
	var nodes [relay.MaxNodes][relay.NodeInfoSize]byte
	tag := func(t []byte) []byte { // the game passes each tag in a 0xFE-byte buffer
		p := make([]byte, 0xFE)
		copy(p, t)
		return p
	}
	r := c.request([]uint32{0x001F0006, staticDesc(relay.NetInfoSize, 1), 0, staticDesc(0xFE, 2), 0,
		staticDesc(0xFE, 3), 0}, b.network.NetInfo[:], tag(b.tag0), tag(b.tag1))
	if r.result() != ResultOK {
		return nodes, r.result()
	}
	if len(r.bufs) != 1 || len(r.bufs[0]) != relay.MaxNodes*relay.NodeInfoSize {
		c.problem("DecryptBeaconData returned %d buffers", len(r.bufs))
		return nodes, r.result()
	}
	for i := range nodes {
		copy(nodes[i][:], r.bufs[0][i*relay.NodeInfoSize:])
	}
	return nodes, ResultOK
}

func (c *ShimConsole) LastScan() []relay.Network {
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []relay.Network
	for _, b := range c.scan {
		out = append(out, b.network)
	}
	return out
}

func (c *ShimConsole) DecryptBeaconData(networkID uint32) ([][relay.NodeInfoSize]byte, uint32) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, b := range c.scan {
		if b.network.NetworkID() != networkID {
			continue
		}
		nodes, r := c.decrypt(b)
		var out [][relay.NodeInfoSize]byte
		for _, n := range nodes {
			if binary.LittleEndian.Uint16(n[0x20:]) != 0 {
				out = append(out, n)
			}
		}
		return out, r
	}
	return nil, ResultOK
}

func (c *ShimConsole) ConnectToNetwork(netInfo, passphrase []byte, connType uint8) uint32 {
	c.mu.Lock()
	defer c.mu.Unlock()
	ni := make([]byte, relay.NetInfoSize)
	copy(ni, netInfo)
	return c.request([]uint32{0x001E0084, uint32(connType), uint32(len(passphrase)), staticDesc(len(ni), 1), 0,
		staticDesc(len(passphrase), 0), 0}, ni, passphrase).result()
}

func (c *ShimConsole) DisconnectNetwork() uint32 { return c.simple(0x000A0000) }

func (c *ShimConsole) EjectClient(node uint16) uint32 { return c.simple(0x00050040, uint32(node)) }

// signals reads the emulator's signal count for an event. Called with c.mu held.
func (c *ShimConsole) signals(h uint32) uint32 {
	resp, err := c.call(fmt.Sprintf("signals %d", h))
	if err != nil {
		c.problem("signals: %v", err)
		return 0
	}
	v, _ := strconv.ParseUint(strings.TrimPrefix(resp, "signals "), 10, 32)
	return uint32(v)
}

// GetConnectionStatus reads the status as the game does. A status that differs from the one
// last read must have been announced on the status event, or the game would never look.
func (c *ShimConsole) GetConnectionStatus() ConnectionStatus {
	c.mu.Lock()
	defer c.mu.Unlock()
	r := c.request([]uint32{0x000B0000})
	if r.result() != ResultOK || len(r.words) != 14 || r.words[0] != 0x000B0340 {
		c.problem("GetConnectionStatus reply %#x", r.words)
		return ConnectionStatus{}
	}
	b := wordsToBytes(r.words[2:14])
	st := ConnectionStatus{
		Status:       binary.LittleEndian.Uint32(b),
		Reason:       binary.LittleEndian.Uint32(b[4:]),
		NodeID:       binary.LittleEndian.Uint16(b[8:]),
		ChangedNodes: binary.LittleEndian.Uint16(b[0xA:]),
		TotalNodes:   b[0x2C],
		MaxNodes:     b[0x2D],
		NodeBitmask:  binary.LittleEndian.Uint16(b[0x2E:]),
	}
	for i := 0; i < relay.MaxNodes; i++ {
		want := uint16(0)
		if st.NodeBitmask&(1<<i) != 0 {
			want = uint16(i + 1)
		}
		if got := binary.LittleEndian.Uint16(b[0xC+2*i:]); got != want {
			c.problem("GetConnectionStatus node list slot %d is %d with bitmask %#04x", i, got, st.NodeBitmask)
			break
		}
	}
	if c.statusEvent != 0 {
		sig := c.signals(c.statusEvent)
		changed := st.Status != c.lastStatus.Status || st.NodeBitmask != c.lastStatus.NodeBitmask || st.ChangedNodes != 0
		if changed && sig == c.lastSignals {
			c.problem("status changed to %+v without a status-event signal", st)
		}
		c.lastSignals = sig
	}
	c.lastStatus = st
	return st
}

func (c *ShimConsole) PeekStatus() ConnectionStatus {
	c.mu.Lock()
	defer c.mu.Unlock()
	resp, err := c.call("peek")
	if err != nil {
		c.problem("peek: %v", err)
		return ConnectionStatus{}
	}
	var st ConnectionStatus
	var node, changed, total, maxNodes, bitmask uint32
	if _, err := fmt.Sscanf(resp, "status %d %d %d %d %d %d %d", &st.Status, &st.Reason, &node, &changed, &total,
		&maxNodes, &bitmask); err != nil {
		c.problem("peek: %q", resp)
	}
	st.NodeID, st.ChangedNodes, st.TotalNodes, st.MaxNodes, st.NodeBitmask =
		uint16(node), uint16(changed), uint8(total), uint8(maxNodes), uint16(bitmask)
	return st
}

func (c *ShimConsole) WaitStatus(ok func(ConnectionStatus) bool, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		if ok(c.PeekStatus()) {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func (c *ShimConsole) Bind(bindID uint32, channel uint8, node uint16, recvBufSize int) uint32 {
	c.mu.Lock()
	defer c.mu.Unlock()
	r := c.request([]uint32{0x00120100, bindID, uint32(recvBufSize), uint32(channel), uint32(node)})
	if r.result() == ResultOK && (len(r.words) != 4 || r.words[0] != 0x00120042 || r.words[2] != 0 || r.words[3] == 0) {
		c.problem("Bind reply %#x carries no event", r.words)
	}
	return r.result()
}

func (c *ShimConsole) Unbind(bindID uint32) uint32 {
	c.mu.Lock()
	defer c.mu.Unlock()
	r := c.request([]uint32{0x00130040, bindID})
	if len(r.words) != 6 || word(r.words, 2) != bindID {
		c.problem("Unbind reply %#x", r.words)
	}
	return r.result()
}

func (c *ShimConsole) SendTo(dst uint16, channel, flags uint8, data []byte) uint32 {
	c.mu.Lock()
	defer c.mu.Unlock()
	aligned := (len(data) + 3) &^ 3
	payload := make([]byte, aligned)
	copy(payload, data)
	return c.request([]uint32{0x00170182, 1, uint32(dst), uint32(channel), 0, uint32(len(data)), uint32(flags),
		staticDesc(aligned, 5), 0}, payload).result()
}

func (c *ShimConsole) PullPacket(bindID uint32, maxSize int) (Packet, bool, uint32) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if maxSize <= 0 {
		maxSize = 0x5dc
	}
	r := c.request([]uint32{0x001400C0, bindID, uint32((maxSize + 3) / 4), uint32(maxSize)})
	if len(r.words) != 6 || r.words[0] != 0x001400C2 || r.words[4] != staticDesc(0x5C8, 0) || len(r.bufs) != 1 {
		c.problem("PullPacket reply %#x", r.words)
		return Packet{}, false, r.result()
	}
	size := int(r.words[2])
	if r.result() != ResultOK || size == 0 {
		return Packet{}, false, r.result()
	}
	return Packet{Src: uint16(r.words[3]), Data: append([]byte(nil), r.bufs[0][:size]...)}, true, ResultOK
}

func (c *ShimConsole) WaitPacket(bindID uint32, maxSize int, timeout time.Duration) (Packet, bool, uint32) {
	deadline := time.Now().Add(timeout)
	for {
		if n, r := c.PollPacket(bindID); r != ResultOK || n > 0 {
			return c.PullPacket(bindID, maxSize)
		}
		if time.Now().After(deadline) {
			return Packet{}, false, ResultOK
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func (c *ShimConsole) PollPacket(bindID uint32) (int, uint32) {
	c.mu.Lock()
	defer c.mu.Unlock()
	resp, err := c.call(fmt.Sprintf("queued %d", bindID))
	if err != nil {
		c.problem("queued: %v", err)
		return 0, ResultInvalidState
	}
	n, _ := strconv.Atoi(strings.TrimPrefix(resp, "queued "))
	if n < 0 {
		return 0, ResultInvalidState
	}
	return n, ResultOK
}

// Kill ends the process without leaving, like a console losing power. Close does the same: a
// replay only closes a console that is in no network.
func (c *ShimConsole) Kill() { _ = c.Close() }

func (c *ShimConsole) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.exited {
		return nil
	}
	c.exited = true
	_, _ = io.WriteString(c.in, "quit\n")
	c.in.Close()
	done := make(chan error, 1)
	go func() { done <- c.cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			c.problem("emulator exited: %v\n%s", err, c.stderr.String())
		}
		return err
	case <-time.After(5 * time.Second):
		_ = c.cmd.Process.Kill()
		c.problem("emulator did not exit")
		return fmt.Errorf("emulator did not exit")
	}
}

// tailBuffer keeps the last max bytes written to it.
type tailBuffer struct {
	mu  sync.Mutex
	buf []byte
	max int
}

func (t *tailBuffer) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf = append(t.buf, p...)
	if len(t.buf) > t.max {
		t.buf = t.buf[len(t.buf)-t.max:]
	}
	return len(p), nil
}

func (t *tailBuffer) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return string(t.buf)
}

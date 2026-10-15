// Package udsfake is a scripted fake console for testing the relay without hardware. A Console
// answers the nwm::UDS commands Pokémon X/Y uses the way the Phase 4 shim will: it keeps the
// connection state the game polls, turns hosting, scanning, joining and data into relay
// messages, and returns the result codes real hardware returned in recon/traces/battle-hw-01.
//
// It is also a reference model for libudsrelay's state machine; Replay drives it from recorded
// traces and checks every reply against the recording.
package udsfake

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"log/slog"
	"sync"
	"time"
	"unicode/utf16"

	"udsrelay/internal/relay"
)

// Result codes.
const (
	ResultOK = 0
	// ResultInvalidState is what hardware returns for DestroyNetwork with no network up and for
	// PullPacket once the network is gone (battle-hw-01). Azahar returns ResultAzaharNotHosting
	// for the first.
	ResultInvalidState     = 0xC8A113EA
	ResultAzaharNotHosting = 0xC8A111EA
	// ResultConnectFailed is returned when the relay refuses a join or cannot be reached. No
	// trace shows what NWM returns here; this placeholder reuses the invalid-state code until
	// Phase 4 tests a failed join on hardware.
	ResultConnectFailed = ResultInvalidState
	// Cmd0023Word is the extra reply word of unnamed command 0x0023. Its low half is always
	// 0x1fff on hardware; the high half varied per initialisation in battle-hw-01.
	Cmd0023Word = 0x00881fff
	// Channel is the Wi-Fi channel GetChannel reports while in a network (11 on hardware).
	Channel = 11
)

// Connection status values (GetConnectionStatus word 0).
const (
	StatusNotConnected         = 3
	StatusConnectedAsHost      = 6
	StatusConnecting           = 7
	StatusConnectedAsClient    = 9
	StatusConnectedAsSpectator = 10
)

// Status-change reasons. Hardware reports 2 after DisconnectNetwork or DestroyNetwork, where
// Azahar reports 0; its name is not known.
const (
	ReasonNone                  = 0
	ReasonConnectionEstablished = 1
	ReasonLeft                  = 2
	ReasonConnectionLost        = 4
)

// ConnectionStatus mirrors the 0x30-byte GetConnectionStatus reply.
type ConnectionStatus struct {
	Status       uint32
	Reason       uint32
	NodeID       uint16
	ChangedNodes uint16
	TotalNodes   uint8
	MaxNodes     uint8
	NodeBitmask  uint16
}

// Packet is one received data frame.
type Packet struct {
	Src     uint16
	Channel uint8
	Data    []byte
}

type bind struct {
	channel uint8
	node    uint16 // 0xFFFF: from any node
	bufSize int
	queued  int
	queue   []Packet
	signals int
}

// Config sets up a Console.
type Config struct {
	Relay string
	Lobby string
	// Impair degrades the console's link to the relay.
	Impair *relay.Impairment
	// Client tunes retries and keepalives; Lobby, Conn and callbacks are filled in.
	Client relay.Options
	Logger *slog.Logger
}

// Console is one fake 3DS.
type Console struct {
	cl  *relay.Client
	log *slog.Logger

	mu      sync.Mutex
	cond    *sync.Cond
	self    [relay.NodeInfoSize]byte
	inited  bool
	st      ConnectionStatus
	netInfo [relay.NetInfoSize]byte
	nodes   [relay.MaxNodes][relay.NodeInfoSize]byte
	binds   map[uint32]*bind
	scan    []relay.Network
	signals int // status-change event signals
	hosted  []uint32
	dropped int // packets dropped because the bind's buffer was full or nothing was bound
}

// New dials the relay. The shim will do this when the game initialises UDS.
func New(cfg Config) (*Console, error) {
	c := &Console{log: cfg.Logger, binds: map[uint32]*bind{}}
	if c.log == nil {
		c.log = slog.New(slog.NewTextHandler(discardWriter{}, nil))
	}
	c.cond = sync.NewCond(&c.mu)
	c.st.Status = StatusNotConnected
	opts := cfg.Client
	opts.Lobby = cfg.Lobby
	opts.OnPacket = c.onPacket
	opts.OnEvent = c.onEvent
	opts.OnReset = c.onReset
	if opts.Logger == nil {
		opts.Logger = cfg.Logger
	}
	if cfg.Impair != nil {
		conn, err := listenUDP()
		if err != nil {
			return nil, err
		}
		opts.Conn = relay.Impaired(conn, *cfg.Impair)
	}
	cl, err := relay.Dial(cfg.Relay, opts)
	if err != nil {
		return nil, err
	}
	c.cl = cl
	return c, nil
}

// Client is the console's relay connection.
func (c *Console) Client() *relay.Client { return c.cl }

// Close leaves any network and disconnects from the relay.
func (c *Console) Close() error { return c.cl.Close() }

// Kill drops off the relay without a word, like a console losing power.
func (c *Console) Kill() { c.cl.Kill() }

// MakeNodeInfo builds the 0x28-byte NodeInfo a game passes to InitializeWithVersion.
func MakeNodeInfo(username string, friendCodeSeed uint64) [relay.NodeInfoSize]byte {
	var n [relay.NodeInfoSize]byte
	binary.LittleEndian.PutUint64(n[0:], friendCodeSeed)
	for i, u := range utf16.Encode([]rune(username)) {
		if i == 10 {
			break
		}
		binary.LittleEndian.PutUint16(n[8+2*i:], u)
	}
	return n
}

// NodeUsername decodes the username in a NodeInfo.
func NodeUsername(n [relay.NodeInfoSize]byte) string {
	var units []uint16
	for i := 0; i < 10; i++ {
		u := binary.LittleEndian.Uint16(n[8+2*i:])
		if u == 0 {
			break
		}
		units = append(units, u)
	}
	return string(utf16.Decode(units))
}

// MakeNetInfo builds the network struct a game passes to BeginHostingNetwork.
func MakeNetInfo(commID uint32, id8 uint8, maxNodes uint8, attributes uint16, appData []byte) []byte {
	b := make([]byte, relay.NetInfoSize)
	binary.BigEndian.PutUint32(b[0x10:], commID)
	b[0x14] = id8
	binary.BigEndian.PutUint16(b[0x16:], attributes)
	b[0x1D] = maxNodes
	b[0x3F] = byte(len(appData))
	copy(b[0x40:], appData)
	return b
}

// HashPassphrase is what the relay sees instead of the UDS passphrase.
func HashPassphrase(p []byte) [32]byte { return sha256.Sum256(p) }

// InitializeWithVersion records the caller's NodeInfo and resets the connection state.
func (c *Console) InitializeWithVersion(self [relay.NodeInfoSize]byte) uint32 {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.self, c.inited = self, true
	c.st = ConnectionStatus{Status: StatusNotConnected}
	c.binds = map[uint32]*bind{}
	return ResultOK
}

// Shutdown leaves any network and drops all binds.
func (c *Console) Shutdown() uint32 {
	c.mu.Lock()
	inNet := c.inNetwork()
	c.mu.Unlock()
	if inNet {
		_ = c.cl.Leave()
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.inited = false
	c.st = ConnectionStatus{Status: StatusNotConnected, NodeID: c.st.NodeID}
	c.binds = map[uint32]*bind{}
	c.cond.Broadcast()
	return ResultOK
}

// SetProbeResponseParam succeeds, as on hardware (Azahar leaves it unimplemented).
func (c *Console) SetProbeResponseParam() uint32 { return ResultOK }

// Cmd0023 succeeds with one word, as on hardware. What the command does is not known.
func (c *Console) Cmd0023() (uint32, uint32) { return ResultOK, Cmd0023Word }

func (c *Console) inNetwork() bool {
	switch c.st.Status {
	case StatusConnectedAsHost, StatusConnectedAsClient, StatusConnectedAsSpectator:
		return true
	}
	return false
}

func (c *Console) signal() {
	c.signals++
	c.cond.Broadcast()
}

// BeginHostingNetwork registers the network on the relay and becomes node 1.
func (c *Console) BeginHostingNetwork(netInfo []byte, passphrase []byte) uint32 {
	c.mu.Lock()
	if c.inNetwork() {
		c.mu.Unlock()
		_ = c.cl.Leave()
		c.mu.Lock()
	}
	self := c.self
	c.mu.Unlock()
	ni := make([]byte, relay.NetInfoSize)
	copy(ni, netInfo)
	ni[0x06] = Channel
	id, err := c.cl.Host(ni, HashPassphrase(passphrase), self)
	if err != nil {
		c.log.Warn("host failed", "err", err)
		return ResultConnectFailed
	}
	binary.BigEndian.PutUint32(ni[0x18:], id)
	ni[0x1C] = 1
	c.mu.Lock()
	defer c.mu.Unlock()
	copy(c.netInfo[:], ni)
	c.nodes = [relay.MaxNodes][relay.NodeInfoSize]byte{}
	c.nodes[0] = self
	maxNodes := ni[0x1D]
	if maxNodes == 0 || maxNodes > relay.MaxNodes {
		maxNodes = relay.MaxNodes
	}
	c.st = ConnectionStatus{
		Status: StatusConnectedAsHost, Reason: ReasonConnectionEstablished, NodeID: 1,
		ChangedNodes: 1, TotalNodes: 1, MaxNodes: maxNodes, NodeBitmask: 1,
	}
	c.hosted = append(c.hosted, id)
	c.signal()
	return ResultOK
}

// HostedNetworkIDs lists the relay-assigned IDs of every network this console hosted.
func (c *Console) HostedNetworkIDs() []uint32 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]uint32(nil), c.hosted...)
}

// DestroyNetwork closes the hosted network. With none up it fails like hardware does.
func (c *Console) DestroyNetwork() uint32 {
	c.mu.Lock()
	if c.st.Status != StatusConnectedAsHost {
		c.mu.Unlock()
		return ResultInvalidState
	}
	c.mu.Unlock()
	_ = c.cl.Leave()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.notConnected(ReasonLeft)
	return ResultOK
}

// notConnected also wakes every bind's receiver, as NWM does: the game's receive thread waits on
// its bind event and must be woken to see the network go (and on Unbind and Shutdown, which the
// shim's emulator also signals).
func (c *Console) notConnected(reason uint32) {
	c.st = ConnectionStatus{Status: StatusNotConnected, Reason: reason, NodeID: c.st.NodeID}
	for _, b := range c.binds {
		b.signals++
	}
	c.signal()
}

// SetApplicationData replaces the hosted network's app data, byte for byte.
func (c *Console) SetApplicationData(data []byte) uint32 {
	c.mu.Lock()
	host := c.st.Status == StatusConnectedAsHost
	c.mu.Unlock()
	if !host {
		return ResultInvalidState
	}
	if err := c.cl.SetAppData(data); err != nil {
		return ResultInvalidState
	}
	c.mu.Lock()
	clear(c.netInfo[0x40 : 0x40+relay.MaxAppData])
	copy(c.netInfo[0x40:], data)
	c.netInfo[0x3F] = byte(len(data))
	c.mu.Unlock()
	return ResultOK
}

// GetApplicationData returns the current network's app data.
func (c *Console) GetApplicationData() ([]byte, uint32) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.inNetwork() {
		return nil, ResultInvalidState
	}
	n := int(c.netInfo[0x3F])
	return append([]byte(nil), c.netInfo[0x40:0x40+n]...), ResultOK
}

// GetChannel reports the Wi-Fi channel: 11 while in a network, else 0.
func (c *Console) GetChannel() (uint32, uint8) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.inNetwork() {
		return ResultOK, Channel
	}
	return ResultOK, 0
}

// RecvBeaconBroadcastData scans the relay. The shim will turn each network into a synthesised
// 802.11 beacon; the fake returns the networks themselves.
func (c *Console) RecvBeaconBroadcastData(commID uint32, id8 uint8) ([]relay.Network, uint32) {
	nets, err := c.cl.Scan(commID, id8)
	if err != nil {
		c.log.Warn("scan failed", "err", err)
		nets = nil
	}
	c.mu.Lock()
	c.scan = nets
	c.mu.Unlock()
	return nets, ResultOK
}

// DecryptBeaconData returns the node list of a network from the last scan.
func (c *Console) DecryptBeaconData(networkID uint32) ([][relay.NodeInfoSize]byte, uint32) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, n := range c.scan {
		if n.NetworkID() == networkID {
			var out [][relay.NodeInfoSize]byte
			for i := 0; i < relay.MaxNodes; i++ {
				if n.Bitmask&(1<<i) != 0 {
					out = append(out, n.Nodes[i])
				}
			}
			return out, ResultOK
		}
	}
	return nil, ResultOK
}

// LastScan is the result of the latest RecvBeaconBroadcastData.
func (c *Console) LastScan() []relay.Network {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]relay.Network(nil), c.scan...)
}

// ConnectToNetwork joins the network netInfo describes (its network ID picks the room).
func (c *Console) ConnectToNetwork(netInfo []byte, passphrase []byte, connType uint8) uint32 {
	if len(netInfo) < 0x1C {
		return ResultConnectFailed
	}
	c.mu.Lock()
	if c.inNetwork() {
		c.mu.Unlock()
		_ = c.cl.Leave()
		c.mu.Lock()
	}
	self := c.self
	c.mu.Unlock()
	res, err := c.cl.Join(binary.BigEndian.Uint32(netInfo[0x18:]), HashPassphrase(passphrase), connType, self)
	if err != nil {
		c.log.Warn("join failed", "err", err)
		return ResultConnectFailed
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.netInfo = res.Network.NetInfo
	c.nodes = res.Network.Nodes
	status := uint32(StatusConnectedAsClient)
	if connType == relay.ConnSpectator {
		status = StatusConnectedAsSpectator
	}
	c.st = ConnectionStatus{
		Status: status, Reason: ReasonConnectionEstablished, NodeID: res.Node,
		ChangedNodes: res.Network.Bitmask, TotalNodes: res.Network.TotalNodes(),
		MaxNodes: res.Network.MaxNodes(), NodeBitmask: res.Network.Bitmask,
	}
	c.signal()
	return ResultOK
}

// DisconnectNetwork leaves the joined network.
func (c *Console) DisconnectNetwork() uint32 {
	c.mu.Lock()
	if c.st.Status != StatusConnectedAsClient && c.st.Status != StatusConnectedAsSpectator {
		c.mu.Unlock()
		return ResultInvalidState
	}
	c.mu.Unlock()
	_ = c.cl.Leave()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.notConnected(ReasonLeft)
	return ResultOK
}

// EjectClient removes a node from the hosted network; node 0 ejects spectators.
func (c *Console) EjectClient(node uint16) uint32 {
	if err := c.cl.Eject(node); err != nil {
		return ResultInvalidState
	}
	return ResultOK
}

// GetConnectionStatus returns the status and clears the changed-nodes mask, as hardware does.
func (c *Console) GetConnectionStatus() ConnectionStatus {
	c.mu.Lock()
	defer c.mu.Unlock()
	st := c.st
	c.st.ChangedNodes = 0
	return st
}

// PeekStatus returns the status without clearing the changed-nodes mask.
func (c *Console) PeekStatus() ConnectionStatus {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.st
}

// StatusSignals counts status-change event signals so far.
func (c *Console) StatusSignals() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.signals
}

// WaitStatus blocks until ok(status) holds or the timeout passes.
func (c *Console) WaitStatus(ok func(ConnectionStatus) bool, timeout time.Duration) bool {
	return c.wait(timeout, func() bool { return ok(c.st) })
}

func (c *Console) wait(timeout time.Duration, ok func() bool) bool {
	deadline := time.Now().Add(timeout)
	t := time.AfterFunc(timeout, func() {
		c.mu.Lock()
		c.cond.Broadcast()
		c.mu.Unlock()
	})
	defer t.Stop()
	c.mu.Lock()
	defer c.mu.Unlock()
	for !ok() {
		if !time.Now().Before(deadline) {
			return false
		}
		c.cond.Wait()
	}
	return true
}

// NodeInfo returns the NodeInfo of a node in the current network.
func (c *Console) NodeInfo(node uint16) ([relay.NodeInfoSize]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if node == 0 || node > relay.MaxNodes || c.st.NodeBitmask&(1<<(node-1)) == 0 {
		return [relay.NodeInfoSize]byte{}, false
	}
	return c.nodes[node-1], true
}

// Bind opens a receive queue for a data channel. node 0xFFFF receives from any node.
func (c *Console) Bind(bindID uint32, channel uint8, node uint16, recvBufSize int) uint32 {
	c.mu.Lock()
	defer c.mu.Unlock()
	if recvBufSize <= 0 {
		recvBufSize = 0x2e30
	}
	c.binds[bindID] = &bind{channel: channel, node: node, bufSize: recvBufSize}
	return ResultOK
}

// Unbind closes a receive queue. Unknown binds succeed, as in the traces.
func (c *Console) Unbind(bindID uint32) uint32 {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.binds, bindID)
	return ResultOK
}

// SendTo sends one frame to a node or to every node (0xFFFF).
func (c *Console) SendTo(dst uint16, channel, flags uint8, data []byte) uint32 {
	c.mu.Lock()
	ok := c.inNetwork() && c.st.Status != StatusConnectedAsSpectator
	c.mu.Unlock()
	if !ok {
		return ResultInvalidState
	}
	if err := c.cl.Send(dst, channel, flags, data); err != nil && !errors.Is(err, relay.ErrClosed) {
		c.log.Warn("send", "err", err)
	}
	return ResultOK
}

// PullPacket takes the oldest frame from a bind's queue. ok is false when the queue is empty.
// Once the network is gone it fails like hardware does.
func (c *Console) PullPacket(bindID uint32, maxSize int) (p Packet, ok bool, result uint32) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.pull(bindID, maxSize)
}

func (c *Console) pull(bindID uint32, maxSize int) (Packet, bool, uint32) {
	if !c.inNetwork() {
		return Packet{}, false, ResultInvalidState
	}
	b := c.binds[bindID]
	if b == nil {
		return Packet{}, false, ResultInvalidState
	}
	if len(b.queue) == 0 {
		return Packet{}, false, ResultOK
	}
	p := b.queue[0]
	if maxSize > 0 && len(p.Data) > maxSize {
		// What NWM does here is untested; keep the frame and fail.
		return Packet{}, false, ResultInvalidState
	}
	b.queue = b.queue[1:]
	b.queued -= len(p.Data)
	return p, true, ResultOK
}

// WaitPacket is PullPacket that waits up to timeout for a frame.
func (c *Console) WaitPacket(bindID uint32, maxSize int, timeout time.Duration) (Packet, bool, uint32) {
	var (
		p   Packet
		ok  bool
		res uint32
	)
	c.wait(timeout, func() bool {
		p, ok, res = c.pull(bindID, maxSize)
		return ok || res != ResultOK
	})
	return p, ok, res
}

// Dropped counts frames dropped on arrival: no matching bind, or its buffer was full.
func (c *Console) Dropped() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.dropped
}

func (c *Console) onPacket(d relay.Data) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.inNetwork() {
		c.dropped++
		return
	}
	delivered := false
	for _, b := range c.binds {
		if b.channel != d.Channel || (b.node != relay.BroadcastNode && b.node != d.Src) {
			continue
		}
		if b.queued+len(d.Payload) > b.bufSize {
			continue
		}
		b.queue = append(b.queue, Packet{Src: d.Src, Channel: d.Channel, Data: d.Payload})
		b.queued += len(d.Payload)
		b.signals++
		delivered = true
	}
	if !delivered {
		c.dropped++
		return
	}
	c.cond.Broadcast()
}

func (c *Console) onEvent(e relay.NodeEvent) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.inNetwork() || e.NetworkID != binary.BigEndian.Uint32(c.netInfo[0x18:]) {
		return
	}
	switch e.Kind {
	case relay.EventJoined, relay.EventLeft:
		bit := uint16(0)
		if e.Node >= 1 && e.Node <= relay.MaxNodes {
			bit = 1 << (e.Node - 1)
			if e.Kind == relay.EventJoined {
				c.nodes[e.Node-1] = e.Info
			} else {
				c.nodes[e.Node-1] = [relay.NodeInfoSize]byte{}
			}
		}
		c.st.NodeBitmask = e.Bitmask
		c.st.TotalNodes = e.TotalNodes
		c.st.ChangedNodes |= bit
		c.netInfo[0x1C] = e.TotalNodes
		c.signal()
	case relay.EventEjected, relay.EventClosed:
		c.notConnected(ReasonConnectionLost)
	}
}

func (c *Console) onReset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.inNetwork() {
		c.notConnected(ReasonConnectionLost)
	}
}

// PollPacket reports what PullPacket would return, and how many frames are queued, without
// taking a frame.
func (c *Console) PollPacket(bindID uint32) (queued int, result uint32) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.inNetwork() || c.binds[bindID] == nil {
		return 0, ResultInvalidState
	}
	return len(c.binds[bindID].queue), ResultOK
}

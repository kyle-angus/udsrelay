// Package relay implements the UDS relay: the UDP wire protocol, the room server both consoles
// dial outbound, and a Go client for it.
//
// Every datagram starts with a 12-byte header, big-endian, and ends with a 16-byte tag:
//
//	0  magic   "UR"
//	2  version 2
//	3  type    message type (Msg*)
//	4  session console-chosen session ID, non-zero
//	8  seq     sender's sequence number
//	10 ack     the seq this datagram answers (responses, ACK), else 0
//
// Requests from a console (HELLO, HOST_CREATE, HOST_UPDATE, SCAN_REQ, JOIN_REQ, EJECT, LEAVE) are
// retried by the client until the response arrives; the response's ack field carries the request's
// seq and the server answers a repeated seq from a cache, so retries are idempotent. NODE_EVENT is
// the only server-initiated reliable message: the server sends one at a time per session and
// retransmits it until the console returns an ACK. DATA and KEEPALIVE are unreliable.
//
// The tag is the first 16 bytes of HMAC-SHA256 over the rest of the datagram, keyed by the
// lobby key. The key never travels: HELLO names the lobby by LobbyID, and the server only
// serves lobbies it was configured with. A datagram whose tag does not verify is dropped without
// an answer. Traffic is not encrypted, and a captured datagram can be replayed; the tag stops
// forged datagrams and strangers using the relay, which is all v2 aims for.
package relay

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
)

const (
	Version    = 2
	HeaderSize = 12
	// TagSize is the truncated HMAC-SHA256 that ends every datagram.
	TagSize = 16
	// DataHeaderSize is the DATA payload prefix: src node, dest node, channel, flags.
	DataHeaderSize = 6
	// NetInfoSize is the UDS network struct the game passes to BeginHostingNetwork.
	NetInfoSize = 0x108
	// NodeInfoSize is the UDS NodeInfo struct (friend-code seed, username, flags).
	NodeInfoSize = 0x28
	// MaxAppData is the most application data a network struct carries.
	MaxAppData = 0xC8
	MaxNodes   = 16
	// BroadcastNode addresses every other node in the room.
	BroadcastNode = 0xFFFF
	// MaxLobbyKey bounds the lobby key string.
	MaxLobbyKey = 64
	// MaxDatagram is the largest datagram either side reads. UDS frames fit in one Ethernet-MTU
	// datagram up to 1454 bytes of payload; larger ones (UDS allows 0x5C6) rely on IP
	// fragmentation.
	MaxDatagram = 2048
)

var magic = [2]byte{'U', 'R'}

// Message types.
const (
	MsgHello        = 1
	MsgHelloAck     = 2
	MsgHostCreate   = 3
	MsgHostAck      = 4
	MsgHostUpdate   = 5
	MsgUpdateAck    = 6
	MsgScanReq      = 7
	MsgScanResp     = 8
	MsgJoinReq      = 9
	MsgJoinAck      = 10
	MsgNodeEvent    = 11
	MsgData         = 12
	MsgEject        = 13
	MsgEjectAck     = 14
	MsgLeave        = 15
	MsgLeaveAck     = 16
	MsgKeepalive    = 17
	MsgKeepaliveAck = 18
	MsgAck          = 19
	// MsgReset tells a console the server has no session for it (server restart or timeout);
	// the console must HELLO again and treat its network as lost.
	MsgReset = 20
)

var msgNames = map[uint8]string{
	MsgHello: "HELLO", MsgHelloAck: "HELLO_ACK", MsgHostCreate: "HOST_CREATE", MsgHostAck: "HOST_ACK",
	MsgHostUpdate: "HOST_UPDATE", MsgUpdateAck: "UPDATE_ACK", MsgScanReq: "SCAN_REQ", MsgScanResp: "SCAN_RESP",
	MsgJoinReq: "JOIN_REQ", MsgJoinAck: "JOIN_ACK", MsgNodeEvent: "NODE_EVENT", MsgData: "DATA",
	MsgEject: "EJECT", MsgEjectAck: "EJECT_ACK", MsgLeave: "LEAVE", MsgLeaveAck: "LEAVE_ACK",
	MsgKeepalive: "KEEPALIVE", MsgKeepaliveAck: "KEEPALIVE_ACK", MsgAck: "ACK", MsgReset: "RESET",
}

// MsgName names a message type for logs.
func MsgName(t uint8) string {
	if n, ok := msgNames[t]; ok {
		return n
	}
	return fmt.Sprintf("type_%d", t)
}

// Status is the first payload byte of every response.
type Status uint8

const (
	StatusOK Status = iota
	StatusNotFound
	StatusFull
	StatusBadPassphrase
	StatusBadRequest
	StatusNotHost
)

func (s Status) Error() string {
	switch s {
	case StatusOK:
		return "ok"
	case StatusNotFound:
		return "network not found"
	case StatusFull:
		return "network full"
	case StatusBadPassphrase:
		return "passphrase mismatch"
	case StatusBadRequest:
		return "bad request"
	case StatusNotHost:
		return "not the host"
	}
	return fmt.Sprintf("status %d", uint8(s))
}

// Node event kinds.
const (
	EventJoined = 1
	EventLeft   = 2
	// EventEjected tells a console the host removed it; its peers see EventLeft.
	EventEjected = 3
	// EventClosed tells a console the host left and the network is gone.
	EventClosed = 4
)

// Leave reasons, carried with EventLeft for diagnostics.
const (
	ReasonClean   = 0
	ReasonTimeout = 1
	ReasonEjected = 2
)

// Connection types for JOIN_REQ, as ConnectToNetwork passes them.
const (
	ConnClient    = 1
	ConnSpectator = 2
)

// Header is the fixed datagram prefix.
type Header struct {
	Type    uint8
	Session uint32
	Seq     uint16
	Ack     uint16
}

var errShort = errors.New("short datagram")

// ParseHeader splits a datagram into header and payload.
func ParseHeader(b []byte) (Header, []byte, error) {
	if len(b) < HeaderSize {
		return Header{}, nil, errShort
	}
	if b[0] != magic[0] || b[1] != magic[1] {
		return Header{}, nil, errors.New("bad magic")
	}
	if b[2] != Version {
		return Header{}, nil, fmt.Errorf("version %d", b[2])
	}
	h := Header{
		Type:    b[3],
		Session: binary.BigEndian.Uint32(b[4:]),
		Seq:     binary.BigEndian.Uint16(b[8:]),
		Ack:     binary.BigEndian.Uint16(b[10:]),
	}
	return h, b[HeaderSize:], nil
}

// AppendHeader appends h to b.
func AppendHeader(b []byte, h Header) []byte {
	b = append(b, magic[0], magic[1], Version, h.Type)
	b = binary.BigEndian.AppendUint32(b, h.Session)
	b = binary.BigEndian.AppendUint16(b, h.Seq)
	return binary.BigEndian.AppendUint16(b, h.Ack)
}

// LobbyID names a lobby in HELLO without revealing its key.
func LobbyID(key string) uint32 {
	sum := sha256.Sum256([]byte("udsrelay lobby id\x00" + key))
	return binary.BigEndian.Uint32(sum[:4])
}

func tag(key, b []byte) []byte {
	m := hmac.New(sha256.New, key)
	m.Write(b)
	return m.Sum(nil)[:TagSize]
}

// Seal returns a copy of datagram b with its tag appended.
func Seal(key, b []byte) []byte {
	out := make([]byte, 0, len(b)+TagSize)
	return append(append(out, b...), tag(key, b)...)
}

// Open checks a sealed datagram's tag and returns the datagram without it.
func Open(key, b []byte) ([]byte, bool) {
	if len(b) < HeaderSize+TagSize {
		return nil, false
	}
	body := b[:len(b)-TagSize]
	return body, hmac.Equal(tag(key, body), b[len(b)-TagSize:])
}

// Packet encodes a header and payload into one datagram.
func Packet(h Header, payload []byte) []byte {
	return append(AppendHeader(make([]byte, 0, HeaderSize+len(payload)), h), payload...)
}

// seqAfter reports whether a comes after b in uint16 serial-number order.
func seqAfter(a, b uint16) bool { return int16(a-b) > 0 }

// reader walks a payload; any short read sets err and returns zeros.
type reader struct {
	b   []byte
	err error
}

func (r *reader) take(n int) []byte {
	if r.err != nil || len(r.b) < n {
		r.err = errShort
		return make([]byte, n)
	}
	v := r.b[:n]
	r.b = r.b[n:]
	return v
}

func (r *reader) u8() uint8   { return r.take(1)[0] }
func (r *reader) u16() uint16 { return binary.BigEndian.Uint16(r.take(2)) }
func (r *reader) u32() uint32 { return binary.BigEndian.Uint32(r.take(4)) }
func (r *reader) u64() uint64 { return binary.BigEndian.Uint64(r.take(8)) }

func (r *reader) nodeInfo() (n [NodeInfoSize]byte) {
	copy(n[:], r.take(NodeInfoSize))
	return n
}

// Network is one hosted network as the relay describes it in SCAN_RESP and JOIN_ACK.
type Network struct {
	// NetInfo is the host's network struct with the relay's network ID and node count filled in.
	NetInfo [NetInfoSize]byte
	Bitmask uint16
	// Nodes holds NodeInfo for each set bit of Bitmask; index node-1.
	Nodes [MaxNodes][NodeInfoSize]byte
}

// Network struct fields the relay reads or writes. Multi-byte fields are big-endian, as games
// pass them.
const (
	niChannel     = 0x06
	niCommID      = 0x10
	niID8         = 0x14
	niAttributes  = 0x16
	niNetworkID   = 0x18
	niTotalNodes  = 0x1C
	niMaxNodes    = 0x1D
	niAppDataSize = 0x3F
	niAppData     = 0x40
)

func (n *Network) CommID() uint32    { return binary.BigEndian.Uint32(n.NetInfo[niCommID:]) }
func (n *Network) ID8() uint8        { return n.NetInfo[niID8] }
func (n *Network) NetworkID() uint32 { return binary.BigEndian.Uint32(n.NetInfo[niNetworkID:]) }
func (n *Network) MaxNodes() uint8   { return n.NetInfo[niMaxNodes] }
func (n *Network) TotalNodes() uint8 { return n.NetInfo[niTotalNodes] }

// AppData returns the network's application data.
func (n *Network) AppData() []byte {
	size := int(n.NetInfo[niAppDataSize])
	if size > MaxAppData {
		size = MaxAppData
	}
	return n.NetInfo[niAppData : niAppData+size]
}

func appendNetwork(b []byte, n *Network) []byte {
	b = append(b, n.NetInfo[:]...)
	b = binary.BigEndian.AppendUint16(b, n.Bitmask)
	for i := 0; i < MaxNodes; i++ {
		if n.Bitmask&(1<<i) != 0 {
			b = append(b, n.Nodes[i][:]...)
		}
	}
	return b
}

func (r *reader) network() Network {
	var n Network
	copy(n.NetInfo[:], r.take(NetInfoSize))
	n.Bitmask = r.u16()
	for i := 0; i < MaxNodes; i++ {
		if n.Bitmask&(1<<i) != 0 {
			n.Nodes[i] = r.nodeInfo()
		}
	}
	return n
}

// NodeEvent is a NODE_EVENT payload.
type NodeEvent struct {
	Kind   uint8
	Reason uint8
	// NetworkID names the network the event is about, so a console can ignore a late event
	// from a network it already left (PSS opens a new one every few seconds).
	NetworkID  uint32
	Node       uint16
	Bitmask    uint16
	TotalNodes uint8
	// Info is the joining node's NodeInfo; zero for other kinds.
	Info [NodeInfoSize]byte
}

func appendNodeEvent(b []byte, e NodeEvent) []byte {
	b = append(b, e.Kind, e.Reason)
	b = binary.BigEndian.AppendUint32(b, e.NetworkID)
	b = binary.BigEndian.AppendUint16(b, e.Node)
	b = binary.BigEndian.AppendUint16(b, e.Bitmask)
	b = append(b, e.TotalNodes)
	return append(b, e.Info[:]...)
}

func parseNodeEvent(p []byte) (NodeEvent, error) {
	r := reader{b: p}
	e := NodeEvent{Kind: r.u8(), Reason: r.u8(), NetworkID: r.u32(), Node: r.u16(), Bitmask: r.u16(), TotalNodes: r.u8(), Info: r.nodeInfo()}
	return e, r.err
}

// Data is a DATA payload.
type Data struct {
	Src, Dst uint16
	Channel  uint8
	Flags    uint8
	Payload  []byte
}

func appendData(b []byte, d Data) []byte {
	b = binary.BigEndian.AppendUint16(b, d.Src)
	b = binary.BigEndian.AppendUint16(b, d.Dst)
	b = append(b, d.Channel, d.Flags)
	return append(b, d.Payload...)
}

func parseData(p []byte) (Data, error) {
	if len(p) < DataHeaderSize {
		return Data{}, errShort
	}
	return Data{
		Src:     binary.BigEndian.Uint16(p),
		Dst:     binary.BigEndian.Uint16(p[2:]),
		Channel: p[4],
		Flags:   p[5],
		Payload: p[DataHeaderSize:],
	}, nil
}

// Host-update kinds.
const (
	UpdateAppData    = 1
	UpdateAttributes = 2
)

// discard is a slog handler that drops everything (slog.DiscardHandler needs Go 1.24; go.mod
// says 1.23).
type discard struct{}

func (discard) Enabled(context.Context, slog.Level) bool  { return false }
func (discard) Handle(context.Context, slog.Record) error { return nil }
func (d discard) WithAttrs([]slog.Attr) slog.Handler      { return d }
func (d discard) WithGroup(string) slog.Handler           { return d }

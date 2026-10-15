package udstrace

import (
	"encoding/binary"
	"fmt"
	"strings"
	"unicode/utf16"
)

// Command IDs for nwm::UDS, as dispatched by Azahar (src/core/hle/service/nwm/nwm_uds.cpp).
const (
	CmdInitializeDeprecated      = 0x0001
	CmdShutdown                  = 0x0003
	CmdBeginHostingDeprecated    = 0x0004
	CmdEjectClient               = 0x0005
	CmdEjectSpectators           = 0x0006
	CmdUpdateNetworkAttribute    = 0x0007
	CmdDestroyNetwork            = 0x0008
	CmdConnectDeprecated         = 0x0009
	CmdDisconnectNetwork         = 0x000A
	CmdGetConnectionStatus       = 0x000B
	CmdGetNodeInformation        = 0x000D
	CmdDecryptBeaconDeprecated   = 0x000E
	CmdRecvBeaconBroadcastData   = 0x000F
	CmdSetApplicationData        = 0x0010
	CmdGetApplicationData        = 0x0011
	CmdBind                      = 0x0012
	CmdUnbind                    = 0x0013
	CmdPullPacket                = 0x0014
	CmdSendTo                    = 0x0017
	CmdGetChannel                = 0x001A
	CmdInitializeWithVersion     = 0x001B
	CmdBeginHostingNetwork       = 0x001D
	CmdConnectToNetwork          = 0x001E
	CmdDecryptBeaconData         = 0x001F
	PortUDS                      = "nwm::UDS"
	PortNDM                      = "ndm:u"
	ndmEnterExclusiveState       = 0x0001
	ndmLeaveExclusiveState       = 0x0002
	ndmQueryExclusiveMode        = 0x0003
	broadcastNode                = 0xFFFF
	networkInfoAppDataSizeOffset = 0x3F
	networkInfoAppDataOffset     = 0x40
)

var udsNames = map[uint16]string{
	0x0001: "Initialize (deprecated)", 0x0003: "Shutdown", 0x0004: "BeginHostingNetwork (deprecated)",
	0x0005: "EjectClient", 0x0006: "EjectSpectators", 0x0007: "UpdateNetworkAttribute",
	0x0008: "DestroyNetwork", 0x0009: "ConnectToNetwork (deprecated)", 0x000A: "DisconnectNetwork",
	0x000B: "GetConnectionStatus", 0x000D: "GetNodeInformation", 0x000E: "DecryptBeaconData (deprecated)",
	0x000F: "RecvBeaconBroadcastData", 0x0010: "SetApplicationData", 0x0011: "GetApplicationData",
	0x0012: "Bind", 0x0013: "Unbind", 0x0014: "PullPacket", 0x0015: "SetMaxSendDelay", 0x0017: "SendTo",
	0x001A: "GetChannel", 0x001B: "InitializeWithVersion", 0x001D: "BeginHostingNetwork",
	0x001E: "ConnectToNetwork", 0x001F: "DecryptBeaconData", 0x0020: "Flush",
	0x0021: "SetProbeResponseParam", 0x0022: "ScanOnConnection",
}

// CommandName names a command when the log line did not carry one (unimplemented commands).
func CommandName(port string, cmd uint16) string {
	if port == PortUDS {
		if n, ok := udsNames[cmd]; ok {
			return n
		}
	}
	return fmt.Sprintf("cmd_%04X", cmd)
}

var exclusiveStates = map[uint32]string{
	0: "None", 1: "Infrastructure", 2: "LocalCommunications", 3: "StreetPass", 4: "StreetPassData",
}

var networkStatuses = map[uint32]string{
	3: "NotConnected", 6: "ConnectedAsHost", 7: "Connecting", 9: "ConnectedAsClient", 10: "ConnectedAsSpectator",
}

var statusReasons = map[uint32]string{0: "None", 1: "ConnectionEstablished", 4: "ConnectionLost"}

// ResultString renders a 3DS result code.
func ResultString(code uint32) string {
	if code == 0 {
		return "OK"
	}
	return fmt.Sprintf("0x%08X (desc=%d module=%d summary=%d level=%d)",
		code, code&0x3FF, (code>>10)&0xFF, (code>>21)&0x3F, (code>>27)&0x1F)
}

// NetworkInfo is the decoded 0x108-byte (or deprecated 0x3C-byte) UDS network struct.
type NetworkInfo struct {
	HostMAC     string
	Channel     uint8
	WlanCommID  uint32
	ID8         uint8
	Attributes  uint16
	NetworkID   uint32
	TotalNodes  uint8
	MaxNodes    uint8
	AppDataSize uint8
	AppData     []byte
}

// ParseNetworkInfo decodes the struct; multi-byte IDs are big-endian as games pass them.
func ParseNetworkInfo(b []byte) (NetworkInfo, error) {
	if len(b) < 0x3C {
		return NetworkInfo{}, fmt.Errorf("network info too short: %d bytes", len(b))
	}
	ni := NetworkInfo{
		HostMAC:    fmt.Sprintf("%02x:%02x:%02x:%02x:%02x:%02x", b[0], b[1], b[2], b[3], b[4], b[5]),
		Channel:    b[6],
		WlanCommID: binary.BigEndian.Uint32(b[0x10:]),
		ID8:        b[0x14],
		Attributes: binary.BigEndian.Uint16(b[0x16:]),
		NetworkID:  binary.BigEndian.Uint32(b[0x18:]),
		TotalNodes: b[0x1C],
		MaxNodes:   b[0x1D],
	}
	if len(b) > networkInfoAppDataSizeOffset {
		ni.AppDataSize = b[networkInfoAppDataSizeOffset]
		end := networkInfoAppDataOffset + int(ni.AppDataSize)
		if end <= len(b) {
			ni.AppData = b[networkInfoAppDataOffset:end]
		}
	}
	return ni, nil
}

// ConnectionStatus is the 0x30-byte GetConnectionStatus reply.
type ConnectionStatus struct {
	Status       uint32
	Reason       uint32
	NodeID       uint16
	ChangedNodes uint16
	Nodes        [16]uint16
	TotalNodes   uint8
	MaxNodes     uint8
	NodeBitmask  uint16
}

// ParseConnectionStatus decodes reply words 2..13 (after header and result).
func ParseConnectionStatus(reply []uint32) (ConnectionStatus, bool) {
	if len(reply) < 14 {
		return ConnectionStatus{}, false
	}
	b := wordsToBytes(reply[2:14])
	cs := ConnectionStatus{
		Status:       binary.LittleEndian.Uint32(b[0:]),
		Reason:       binary.LittleEndian.Uint32(b[4:]),
		NodeID:       binary.LittleEndian.Uint16(b[8:]),
		ChangedNodes: binary.LittleEndian.Uint16(b[0xA:]),
		TotalNodes:   b[0x2C],
		MaxNodes:     b[0x2D],
		NodeBitmask:  binary.LittleEndian.Uint16(b[0x2E:]),
	}
	for i := range cs.Nodes {
		cs.Nodes[i] = binary.LittleEndian.Uint16(b[0xC+2*i:])
	}
	return cs, true
}

func (cs ConnectionStatus) String() string {
	status := networkStatuses[cs.Status]
	if status == "" {
		status = fmt.Sprintf("status_%d", cs.Status)
	}
	reason := statusReasons[cs.Reason]
	if reason == "" {
		reason = fmt.Sprintf("reason_%d", cs.Reason)
	}
	return fmt.Sprintf("%s reason=%s node=%d nodes=%d/%d bitmask=0x%04x changed=0x%04x",
		status, reason, cs.NodeID, cs.TotalNodes, cs.MaxNodes, cs.NodeBitmask, cs.ChangedNodes)
}

func wordsToBytes(words []uint32) []byte {
	b := make([]byte, 4*len(words))
	for i, w := range words {
		binary.LittleEndian.PutUint32(b[4*i:], w)
	}
	return b
}

// nodeUsername decodes the UTF-16 username from InitializeWithVersion request words 2..11.
func nodeUsername(req []uint32) string {
	if len(req) < 12 {
		return ""
	}
	b := wordsToBytes(req[2:12])
	units := make([]uint16, 0, 10)
	for i := 0; i < 10; i++ {
		u := binary.LittleEndian.Uint16(b[8+2*i:])
		if u == 0 {
			break
		}
		units = append(units, u)
	}
	return string(utf16.Decode(units))
}

func word(ws []uint32, i int) uint32 {
	if i < len(ws) {
		return ws[i]
	}
	return 0
}

func nodeString(n uint32) string {
	if n&0xFFFF == broadcastNode {
		return "broadcast"
	}
	return fmt.Sprintf("%d", n&0xFFFF)
}

// Args summarises a call's request arguments.
func Args(c *Call) string {
	w := c.Req.Words
	switch c.Port {
	case PortUDS:
		switch c.Cmd {
		case CmdInitializeWithVersion:
			return fmt.Sprintf("shmem=%#x user=%q version=%#x", word(w, 1), nodeUsername(w), word(w, 12)&0xFFFF)
		case CmdInitializeDeprecated:
			return fmt.Sprintf("shmem=%#x user=%q", word(w, 1), nodeUsername(w))
		case CmdRecvBeaconBroadcastData:
			return fmt.Sprintf("out_size=%#x comm_id=0x%08x id8=0x%02x", word(w, 1), word(w, 15), word(w, 16))
		case CmdGetNodeInformation, CmdEjectClient:
			return "node=" + nodeString(word(w, 1))
		case CmdBind:
			return fmt.Sprintf("bind=%d recv_buf=%#x chan=%d node=%s", word(w, 1), word(w, 2), word(w, 3)&0xFF, nodeString(word(w, 4)))
		case CmdUnbind:
			return fmt.Sprintf("bind=%d", word(w, 1))
		case CmdUpdateNetworkAttribute:
			return fmt.Sprintf("bitmask=0x%04x flag=%d", word(w, 1)&0xFFFF, word(w, 2)&0xFF)
		case CmdSendTo:
			return fmt.Sprintf("dest=%s chan=%d size=%d flags=0x%02x", nodeString(word(w, 2)), word(w, 3)&0xFF, word(w, 5), word(w, 6)&0xFF)
		case CmdPullPacket:
			return fmt.Sprintf("bind=%d max=%#x", word(w, 1), word(w, 3))
		case CmdBeginHostingNetwork, CmdBeginHostingDeprecated, CmdConnectToNetwork, CmdConnectDeprecated:
			return networkArgs(c)
		case CmdSetApplicationData:
			return fmt.Sprintf("size=%d", word(w, 1))
		case CmdGetApplicationData:
			return fmt.Sprintf("max=%d", word(w, 1))
		}
	case PortNDM:
		switch c.Cmd {
		case ndmEnterExclusiveState:
			return "state=" + exclusiveStateName(word(w, 1))
		}
	}
	if len(w) > 1 {
		return "words=" + hexWords(w[1:])
	}
	return ""
}

func exclusiveStateName(s uint32) string {
	if n, ok := exclusiveStates[s]; ok {
		return n
	}
	return fmt.Sprintf("%d", s)
}

func networkArgs(c *Call) string {
	var parts []string
	if c.Cmd == CmdConnectToNetwork || c.Cmd == CmdConnectDeprecated {
		if b := c.Blob("netinfo"); b != nil && b.Extra["conn_type"] != "" {
			parts = append(parts, "conn_type="+b.Extra["conn_type"])
		}
	}
	if b := c.Blob("netinfo"); b != nil {
		if ni, err := ParseNetworkInfo(b.Data); err == nil {
			parts = append(parts, fmt.Sprintf("comm_id=0x%08x id8=0x%02x max_nodes=%d appdata=%dB",
				ni.WlanCommID, ni.ID8, ni.MaxNodes, ni.AppDataSize))
		}
	}
	if b := c.Blob("passphrase"); b != nil {
		parts = append(parts, fmt.Sprintf("passphrase=%dB", b.Size))
	}
	return strings.Join(parts, " ")
}

// ReplySummary summarises a call's reply.
func ReplySummary(c *Call) string {
	if c.Unimplemented {
		return "UNIMPLEMENTED in Azahar"
	}
	code, ok := c.Result()
	if !ok {
		if c.AwaitingWake {
			return "(no wake-up logged)"
		}
		return "(no reply logged)"
	}
	res := ResultString(code)
	if code != 0 {
		return res
	}
	r := c.Reply
	switch c.Port {
	case PortUDS:
		switch c.Cmd {
		case CmdGetConnectionStatus:
			if cs, ok := ParseConnectionStatus(r); ok {
				return res + " " + cs.String()
			}
		case CmdPullPacket:
			if word(r, 2) == 0 {
				return res + " empty"
			}
			return fmt.Sprintf("%s size=%d src=%d", res, word(r, 2), word(r, 3)&0xFFFF)
		case CmdGetChannel:
			return fmt.Sprintf("%s channel=%d", res, word(r, 2))
		case CmdGetApplicationData:
			return fmt.Sprintf("%s size=%d", res, word(r, 2))
		case CmdBind, CmdInitializeWithVersion, CmdInitializeDeprecated:
			return res + " +event handle"
		}
	case PortNDM:
		if c.Cmd == ndmQueryExclusiveMode {
			return res + " state=" + exclusiveStateName(word(r, 2))
		}
	}
	if len(r) > 2 {
		return res + " words=" + hexWords(r[2:])
	}
	return res
}

func hexWords(ws []uint32) string {
	s := make([]string, len(ws))
	for i, w := range ws {
		s[i] = fmt.Sprintf("%#x", w)
	}
	return strings.Join(s, ",")
}

// Describe renders a call as one timeline line.
func Describe(c *Call) string {
	prefix := c.Name
	if c.Port != PortUDS {
		prefix = c.Port + " " + c.Name
	}
	return fmt.Sprintf("%s(%s) -> %s", prefix, Args(c), ReplySummary(c))
}

package relay

import (
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// Options configure a Client. Callbacks run on the client's read goroutine; they must not block
// or call back into the client's request methods.
type Options struct {
	// Lobby is the lobby key, shared with the relay's lobby list and the other console. It
	// authenticates every datagram and is never sent.
	Lobby string
	// SessionID is chosen at random when zero.
	SessionID uint32
	// Retry is the request retransmit interval. Default 250 ms.
	Retry time.Duration
	// RequestTimeout bounds one request including retries. Default 3 s.
	RequestTimeout time.Duration
	// Keepalive overrides the interval the server advertises.
	Keepalive time.Duration
	// Conn is the socket to use, e.g. an Impaired conn; by default the client binds a UDP port.
	Conn net.PacketConn

	OnPacket func(Data)
	OnEvent  func(NodeEvent)
	// OnReset runs when the server says it has no session for us; the client says HELLO again
	// on its own, but any network it hosted or joined is gone.
	OnReset func()
	Logger  *slog.Logger
}

// Client is one console's connection to the relay.
type Client struct {
	conn   net.PacketConn
	server net.Addr
	opts   Options
	log    *slog.Logger
	id     uint32
	key    []byte

	reqMu sync.Mutex // one request in flight at a time

	mu      sync.Mutex
	seq     uint16
	waitSeq uint16
	waitCh  chan []byte // responses to the request in flight
	evLast  uint16
	evAny   bool
	closed  bool

	rtt       atomic.Int64
	keepalive time.Duration
	done      chan struct{}
	wg        sync.WaitGroup
}

// ErrTimeout means the relay did not answer a request in time.
var ErrTimeout = errors.New("relay did not answer")

// ErrClosed means the client was closed.
var ErrClosed = errors.New("relay client closed")

// Dial connects to the relay at addr and says HELLO.
func Dial(addr string, opts Options) (*Client, error) {
	if len(opts.Lobby) == 0 || len(opts.Lobby) > MaxLobbyKey {
		return nil, fmt.Errorf("lobby key must be 1 to %d bytes", MaxLobbyKey)
	}
	server, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, err
	}
	if opts.Retry == 0 {
		opts.Retry = 250 * time.Millisecond
	}
	if opts.RequestTimeout == 0 {
		opts.RequestTimeout = 3 * time.Second
	}
	if opts.Logger == nil {
		opts.Logger = slog.New(discard{})
	}
	conn := opts.Conn
	if conn == nil {
		if conn, err = net.ListenPacket("udp", ":0"); err != nil {
			return nil, err
		}
	}
	id := opts.SessionID
	for id == 0 {
		id = rand.Uint32()
	}
	c := &Client{conn: conn, server: server, opts: opts, log: opts.Logger, id: id, key: []byte(opts.Lobby),
		done: make(chan struct{})}
	c.wg.Add(1)
	go c.readLoop()
	if err := c.hello(); err != nil {
		c.Kill()
		return nil, err
	}
	c.wg.Add(1)
	go c.keepaliveLoop()
	return c, nil
}

// SessionID is this client's session.
func (c *Client) SessionID() uint32 { return c.id }

// LocalAddr is the client's socket address.
func (c *Client) LocalAddr() net.Addr { return c.conn.LocalAddr() }

// RTT is the latest keepalive round trip, or 0 before the first.
func (c *Client) RTT() time.Duration { return time.Duration(c.rtt.Load()) }

func (c *Client) hello() error {
	p := binary.BigEndian.AppendUint32(nil, LobbyID(c.opts.Lobby))
	resp, err := c.request(MsgHello, MsgHelloAck, p)
	if err != nil {
		return err
	}
	r := reader{b: resp[1:]}
	ka := time.Duration(r.u16()) * time.Millisecond
	if c.opts.Keepalive != 0 {
		ka = c.opts.Keepalive
	}
	if ka <= 0 {
		ka = time.Second
	}
	c.mu.Lock()
	c.keepalive = ka
	c.mu.Unlock()
	return nil
}

// Host registers a network: the game's 0x108-byte network struct, the SHA-256 of its passphrase,
// and the host's NodeInfo. It returns the network ID the relay assigned.
func (c *Client) Host(netInfo []byte, passHash [32]byte, self [NodeInfoSize]byte) (uint32, error) {
	if len(netInfo) != NetInfoSize {
		return 0, fmt.Errorf("network struct is %d bytes, want %d", len(netInfo), NetInfoSize)
	}
	p := append(append(append([]byte{}, passHash[:]...), self[:]...), netInfo...)
	resp, err := c.request(MsgHostCreate, MsgHostAck, p)
	if err != nil {
		return 0, err
	}
	r := reader{b: resp[1:]}
	id := r.u32()
	return id, r.err
}

// SetAppData replaces the hosted network's application data.
func (c *Client) SetAppData(data []byte) error {
	if len(data) > MaxAppData {
		return fmt.Errorf("app data is %d bytes, max %d", len(data), MaxAppData)
	}
	_, err := c.request(MsgHostUpdate, MsgUpdateAck, append([]byte{UpdateAppData, byte(len(data))}, data...))
	return err
}

// SetAttributes replaces the hosted network's attribute word.
func (c *Client) SetAttributes(a uint16) error {
	_, err := c.request(MsgHostUpdate, MsgUpdateAck, binary.BigEndian.AppendUint16([]byte{UpdateAttributes}, a))
	return err
}

// Scan lists networks in the lobby matching commID (0xFFFFFFFF for any) and id8 (0xFF for any),
// excluding our own. A network whose datagram was lost is missing, as on the air.
func (c *Client) Scan(commID uint32, id8 uint8) ([]Network, error) {
	p := append(binary.BigEndian.AppendUint32(nil, commID), id8)
	var nets []Network
	got := map[uint8]bool{}
	err := c.requestFunc(MsgScanReq, p, func(t uint8, resp []byte) (bool, error) {
		if t != MsgScanResp {
			return false, nil
		}
		if Status(resp[0]) != StatusOK {
			return true, Status(resp[0])
		}
		r := reader{b: resp[1:]}
		idx, total := r.u8(), r.u8()
		if total == 0 {
			return true, nil
		}
		n := r.network()
		if r.err == nil && !got[idx] {
			got[idx] = true
			nets = append(nets, n)
		}
		return len(got) == int(total), nil
	}, true, c.opts.RequestTimeout)
	return nets, err
}

// JoinResult is a successful JOIN_ACK.
type JoinResult struct {
	Node    uint16 // 0 for a spectator
	Network Network
}

// Join joins network id as a client (ConnClient) or spectator (ConnSpectator).
func (c *Client) Join(networkID uint32, passHash [32]byte, connType uint8, self [NodeInfoSize]byte) (JoinResult, error) {
	p := binary.BigEndian.AppendUint32(nil, networkID)
	p = append(append(append(p, passHash[:]...), connType), self[:]...)
	resp, err := c.request(MsgJoinReq, MsgJoinAck, p)
	if err != nil {
		return JoinResult{}, err
	}
	r := reader{b: resp[1:]}
	res := JoinResult{Node: r.u16(), Network: r.network()}
	return res, r.err
}

// Eject removes a client node from our hosted network; node 0 removes every spectator.
func (c *Client) Eject(node uint16) error {
	_, err := c.request(MsgEject, MsgEjectAck, binary.BigEndian.AppendUint16(nil, node))
	return err
}

// Leave leaves the network we host or joined. Hosting a network closes it.
func (c *Client) Leave() error {
	_, err := c.request(MsgLeave, MsgLeaveAck, nil)
	return err
}

// Send sends one data frame. It is unreliable, like UDS.
func (c *Client) Send(dst uint16, channel, flags uint8, payload []byte) error {
	return c.send(MsgData, appendData(make([]byte, 0, DataHeaderSize+len(payload)), Data{Dst: dst, Channel: channel, Flags: flags, Payload: payload}), 0)
}

// Close leaves any network (best effort) and closes the socket.
func (c *Client) Close() error {
	c.mu.Lock()
	closed := c.closed
	c.mu.Unlock()
	if closed {
		return nil
	}
	_ = c.requestFunc(MsgLeave, nil, func(t uint8, _ []byte) (bool, error) { return t == MsgLeaveAck, nil }, false, 500*time.Millisecond)
	c.Kill()
	return nil
}

// Kill drops the connection without telling the relay, as a console that loses power or Wi-Fi
// would. The relay notices after its timeout.
func (c *Client) Kill() {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	close(c.done)
	c.mu.Unlock()
	c.conn.Close()
	c.wg.Wait()
}

func (c *Client) send(typ uint8, payload []byte, seq uint16) error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return ErrClosed
	}
	if seq == 0 {
		c.seq++
		if c.seq == 0 {
			c.seq++
		}
		seq = c.seq
	}
	c.mu.Unlock()
	_, err := c.conn.WriteTo(Seal(c.key, Packet(Header{Type: typ, Session: c.id, Seq: seq}, payload)), c.server)
	return err
}

func (c *Client) request(typ, respType uint8, payload []byte) ([]byte, error) {
	var out []byte
	err := c.requestFunc(typ, payload, func(t uint8, resp []byte) (bool, error) {
		if t != respType {
			return false, nil
		}
		if Status(resp[0]) != StatusOK {
			return true, Status(resp[0])
		}
		out = resp
		return true, nil
	}, false, c.opts.RequestTimeout)
	return out, err
}

// requestFunc sends a request and feeds each response to handle until it reports done. A request
// is retried every Retry interval; a multi-datagram response (scan) also returns what it has
// once the timeout passes after at least one datagram arrived.
func (c *Client) requestFunc(typ uint8, payload []byte, handle func(uint8, []byte) (bool, error), partialOK bool, timeout time.Duration) error {
	c.reqMu.Lock()
	defer c.reqMu.Unlock()
	ch := make(chan []byte, 64)
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return ErrClosed
	}
	c.seq++
	if c.seq == 0 {
		c.seq++
	}
	seq := c.seq
	c.waitSeq, c.waitCh = seq, ch
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		c.waitCh = nil
		c.mu.Unlock()
	}()

	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	retry := time.NewTicker(c.opts.Retry)
	defer retry.Stop()
	any := false
	if err := c.send(typ, payload, seq); err != nil {
		return err
	}
	for {
		select {
		case b := <-ch:
			if len(b) < HeaderSize+1 {
				continue
			}
			any = true
			done, err := handle(b[3], b[HeaderSize:])
			if done {
				return err
			}
		case <-retry.C:
			if err := c.send(typ, payload, seq); err != nil {
				return err
			}
		case <-deadline.C:
			if any && partialOK {
				return nil
			}
			return fmt.Errorf("%s: %w", MsgName(typ), ErrTimeout)
		case <-c.done:
			return ErrClosed
		}
	}
}

func (c *Client) readLoop() {
	defer c.wg.Done()
	buf := make([]byte, MaxDatagram)
	for {
		n, addr, err := c.conn.ReadFrom(buf)
		if err != nil {
			select {
			case <-c.done:
				return
			default:
			}
			if errors.Is(err, net.ErrClosed) {
				return
			}
			continue
		}
		if !equalAddr(addr, c.server) {
			continue
		}
		body, ok := Open(c.key, buf[:n])
		if !ok {
			continue // not from our relay, or a different lobby key
		}
		h, p, err := ParseHeader(body)
		if err != nil || h.Session != c.id {
			continue
		}
		switch h.Type {
		case MsgData:
			if d, err := parseData(p); err == nil && c.opts.OnPacket != nil {
				d.Payload = append([]byte(nil), d.Payload...)
				c.opts.OnPacket(d)
			}
		case MsgNodeEvent:
			c.ackEvent(h.Seq)
			c.mu.Lock()
			fresh := !c.evAny || seqAfter(h.Seq, c.evLast)
			if fresh {
				c.evAny, c.evLast = true, h.Seq
			}
			c.mu.Unlock()
			if e, err := parseNodeEvent(p); err == nil && fresh && c.opts.OnEvent != nil {
				c.opts.OnEvent(e)
			}
		case MsgKeepaliveAck:
			if len(p) >= 8 {
				sent := int64(binary.BigEndian.Uint64(p))
				c.rtt.Store(time.Now().UnixNano() - sent)
			}
		case MsgReset:
			c.log.Info("relay reset our session", "session", hex32(c.id))
			c.mu.Lock()
			c.evAny = false
			c.mu.Unlock()
			if c.opts.OnReset != nil {
				c.opts.OnReset()
			}
			go func() { _ = c.hello() }()
		default:
			c.mu.Lock()
			if c.waitCh != nil && h.Ack == c.waitSeq {
				select {
				case c.waitCh <- append([]byte(nil), body...):
				default:
				}
			}
			c.mu.Unlock()
		}
	}
}

func (c *Client) ackEvent(seq uint16) {
	c.mu.Lock()
	c.seq++
	s := c.seq
	c.mu.Unlock()
	_, _ = c.conn.WriteTo(Seal(c.key, Packet(Header{Type: MsgAck, Session: c.id, Seq: s, Ack: seq}, nil)), c.server)
}

func (c *Client) keepaliveLoop() {
	defer c.wg.Done()
	c.mu.Lock()
	ka := c.keepalive
	c.mu.Unlock()
	t := time.NewTicker(ka)
	defer t.Stop()
	for {
		select {
		case <-c.done:
			return
		case <-t.C:
			_ = c.send(MsgKeepalive, binary.BigEndian.AppendUint64(nil, uint64(time.Now().UnixNano())), 0)
		}
	}
}

package relay

import (
	"math/rand/v2"
	"net"
	"sync"
	"time"
)

// Impairment describes a bad link for testing: each datagram is dropped with probability Loss,
// and survivors are delayed by Delay plus up to Jitter. Loss applies in both directions; delay
// applies to sends only, so set it to the full extra round trip you want.
type Impairment struct {
	Loss   float64
	Delay  time.Duration
	Jitter time.Duration
	// Seed makes the loss pattern repeatable when non-zero.
	Seed uint64
}

// Impaired wraps conn with the impairment.
func Impaired(conn net.PacketConn, imp Impairment) net.PacketConn {
	seed := imp.Seed
	if seed == 0 {
		seed = rand.Uint64()
	}
	return &impairedConn{PacketConn: conn, imp: imp, rng: rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))}
}

type impairedConn struct {
	net.PacketConn
	imp Impairment
	mu  sync.Mutex
	rng *rand.Rand
}

func (c *impairedConn) drop() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.imp.Loss > 0 && c.rng.Float64() < c.imp.Loss
}

func (c *impairedConn) delay() time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	d := c.imp.Delay
	if c.imp.Jitter > 0 {
		d += time.Duration(c.rng.Int64N(int64(c.imp.Jitter)))
	}
	return d
}

func (c *impairedConn) WriteTo(b []byte, addr net.Addr) (int, error) {
	if c.drop() {
		return len(b), nil
	}
	d := c.delay()
	if d <= 0 {
		return c.PacketConn.WriteTo(b, addr)
	}
	cp := append([]byte(nil), b...)
	time.AfterFunc(d, func() { _, _ = c.PacketConn.WriteTo(cp, addr) })
	return len(b), nil
}

func (c *impairedConn) ReadFrom(b []byte) (int, net.Addr, error) {
	for {
		n, addr, err := c.PacketConn.ReadFrom(b)
		if err != nil || !c.drop() {
			return n, addr, err
		}
	}
}

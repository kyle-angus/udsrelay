package relay

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"net"
	"os/exec"
	"strconv"
	"sync"
	"testing"
	"time"
)

// TestCClientAgainstRelay runs the shim core's C relay client (shim/core/tests/relay_probe.c)
// against this relay over real UDP, with a Go client as the other console. The C side hosts a
// trade network, the Go side scans and joins it, both exchange frames, and the Go side leaves.
func TestCClientAgainstRelay(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs a C program")
	}
	if _, err := exec.LookPath("make"); err != nil {
		t.Skip("make not available")
	}
	build := exec.Command("make", "-C", "../../shim/core", "build/relay_probe")
	if out, err := build.CombinedOutput(); err != nil {
		t.Skipf("cannot build relay_probe: %v\n%s", err, out)
	}

	const key, frames = "probe-key", 20
	h := newHarness(t, Config{Lobbies: []Lobby{{Name: "probe", Key: key}}})
	_, port, _ := net.SplitHostPort(h.srv.Addr().String())

	probe := exec.Command("../../shim/core/build/relay_probe", "127.0.0.1", port, key, strconv.Itoa(frames))
	var probeOut bytes.Buffer
	probe.Stdout, probe.Stderr = &probeOut, &probeOut
	if err := probe.Start(); err != nil {
		t.Fatal(err)
	}
	probeDone := make(chan error, 1)
	go func() { probeDone <- probe.Wait() }()

	var mu sync.Mutex
	var got []string
	c, err := Dial(h.srv.Addr().String(), Options{Lobby: key, Retry: 50 * time.Millisecond, OnPacket: func(d Data) {
		mu.Lock()
		defer mu.Unlock()
		if d.Src == 1 && d.Channel == 243 && d.Flags == 2 {
			got = append(got, string(d.Payload))
		}
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Kill()

	var netID uint32
	for deadline := time.Now().Add(5 * time.Second); netID == 0 && time.Now().Before(deadline); {
		nets, err := c.Scan(0x00055d10, 0x01)
		if err == nil && len(nets) == 1 {
			netID = nets[0].NetworkID()
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if netID == 0 {
		t.Fatalf("the C client's network never appeared in a scan\nprobe output:\n%s", probeOut.String())
	}
	pass := sha256.Sum256([]byte("GNgBwMcVotYs\x00"))
	var self [NodeInfoSize]byte
	res, err := c.Join(netID, pass, ConnClient, self)
	if err != nil || res.Node != 2 {
		t.Fatalf("join: node %d, %v", res.Node, err)
	}
	for i := 0; i < frames; i++ {
		if err := c.Send(1, 243, 2, []byte(fmt.Sprintf("g2c-%d", i))); err != nil {
			t.Fatal(err)
		}
	}
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		mu.Lock()
		n := len(got)
		mu.Unlock()
		if n >= frames {
			break
		}
	}
	mu.Lock()
	for i := 0; i < frames; i++ {
		if i >= len(got) || got[i] != fmt.Sprintf("c2g-%d", i) {
			t.Errorf("frame %d from the C client: got %q", i, got)
			break
		}
	}
	mu.Unlock()
	if err := c.Leave(); err != nil {
		t.Fatal(err)
	}

	select {
	case err := <-probeDone:
		if err != nil || !bytes.Contains(probeOut.Bytes(), []byte("PASS")) {
			t.Fatalf("relay_probe: %v\n%s", err, probeOut.String())
		}
	case <-time.After(10 * time.Second):
		probe.Process.Kill()
		t.Fatalf("relay_probe did not finish\n%s", probeOut.String())
	}
	t.Logf("relay_probe:\n%s", probeOut.String())
}

package relay

import (
	"encoding/binary"
	"encoding/hex"
	"testing"
)

// The console shim implements the wire format in C (shim/core/src/relay_wire.c). Both sides
// check these vectors, computed independently with Python's hashlib and hmac; see
// test_relay_wire in shim/core/tests/test_core.c.
func TestWireVectors(t *testing.T) {
	const key = "test-key"
	if id := LobbyID(key); id != 0x6ab979dd {
		t.Errorf("LobbyID = %#x, want 0x6ab979dd", id)
	}

	hello := Seal([]byte(key), Packet(Header{Type: MsgHello, Session: 0x01020304, Seq: 7},
		binary.BigEndian.AppendUint32(nil, LobbyID(key))))
	if got, want := hex.EncodeToString(hello), "5552020101020304000700006ab979dd3cc2c53977b4c5f9fb54fe29371441f3"; got != want {
		t.Errorf("HELLO\n got %s\nwant %s", got, want)
	}

	data := Seal([]byte(key), Packet(Header{Type: MsgData, Session: 0x01020304, Seq: 9},
		appendData(nil, Data{Src: 1, Dst: 2, Channel: 243, Flags: 2, Payload: []byte{2, 1, 2, 1}})))
	if got, want := hex.EncodeToString(data), "5552020c010203040009000000010002f3020201020103570ab1ecbb5ebb3e12ef9d770af768"; got != want {
		t.Errorf("DATA\n got %s\nwant %s", got, want)
	}
	if _, ok := Open([]byte(key), data); !ok {
		t.Error("Open rejects its own DATA vector")
	}
}

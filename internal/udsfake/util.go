package udsfake

import "net"

func listenUDP() (net.PacketConn, error) { return net.ListenPacket("udp", ":0") }

type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }

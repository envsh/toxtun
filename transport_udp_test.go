package main

import (
	"log"
	"testing"
)

func TestUdp(t *testing.T) {
	tp := NewDirectUdpTransport()
	log.Println(tp.peerIP, tp.localIP, tp.port)
}

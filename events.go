package main

import (
	"net"
)

// ToxPollEvent
// ToxReadyReadEvent
// KcpPollEvent
// KcpReadyReadEvent
// ClientReadyReadEvent
// NewConnEvent

type ToxPollEvent struct{}
type ToxReadyReadEvent struct {
	friendNumber uint32
	message      string
}
type ToxMessageEvent ToxReadyReadEvent

type KcpPollEvent struct{}
type KcpReadyReadEvent struct {
	ch *Channel
}
type KcpOutputEvent struct {
	buf   []byte
	size  int
	extra interface{}
}

type NewConnEvent struct {
	conn net.Conn
}
type ClientReadyReadEvent struct {
	ch   *Channel
	buf  []byte
	size int
}
type ServerReadyReadEvent ClientReadyReadEvent

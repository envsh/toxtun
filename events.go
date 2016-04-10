package main

import (
	"net"
	"time"
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
type KcpCheckCloseEvent struct{}

type NewConnEvent struct {
	conn  net.Conn
	times int
	btime time.Time
}
type ClientReadyReadEvent struct {
	ch   *Channel
	buf  []byte
	size int
}
type ServerReadyReadEvent ClientReadyReadEvent
type ClientCloseEvent struct {
	ch *Channel
}
type ServerCloseEvent ClientCloseEvent
type ClientCheckACKEvent struct {
	ch *Channel
}

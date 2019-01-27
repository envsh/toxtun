package main

import (
	"gopp"

	"github.com/xtaci/smux"

	rudp "mkuse/rudp2"
)

type muxone struct {
	*smux.Session
	rudp_ *rudp.RUDP

	conv     uint32 // auto increment when reconnect
	writeout func(data *rudp.PfxByteArray, prior bool) error
}

/////
func NewMuxone(conv uint32, writeout func(data *rudp.PfxByteArray, prior bool) error) *muxone {
	this := &muxone{}
	this.conv = conv
	this.writeout = writeout

	this.reset()
	return this
}

func (this *muxone) reset() {
	this.rudp_ = rudp.NewRUDP(this.conv, this.writeout)
	cfg := &smux.Config{}
	cfg = &*smux.DefaultConfig()
	cfg.KeepAliveInterval *= 2
	cfg.KeepAliveTimeout *= 2
	sess, err := smux.Server(this.rudp_, cfg)
	gopp.ErrPrint(err)
	this.Session = sess

	this.OpenStream("hehhehe")
}

func (this *muxone) Close() {
	this.Session.Close()
	this.rudp_.Close()
}

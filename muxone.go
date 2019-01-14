package main

import (
	"gopp"
	"mkuse/rudp"

	"github.com/xtaci/smux"
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
	cfg = smux.DefaultConfig()
	cfg.KeepAliveInterval *= 3
	cfg.KeepAliveTimeout *= 3
	sess, err := smux.Server(this.rudp_, nil)
	gopp.ErrPrint(err)
	this.Session = sess

	this.OpenStream("hehhehe")
}

package main

import (
	"gopp"
	"io"
	"log"
	"time"

	"github.com/hashicorp/yamux"
	// tox "github.com/kitech/go-toxcore"
	tox "github.com/TokTok/go-toxcore-c"
	"github.com/kitech/godsts/maps/hashmap"
)

var unconns = hashmap.New() // tname|global ip(dht node id) => *Unconnection

type Unconnection struct {
	tname  string
	gip    string
	grptp  *TransportGroup
	muxcli *yamux.Session
	kcptp  *KcpTransport

	stms  *hashmap.Map // stmid => *yamux.Stream
	_stms *yamux.Session
}

func newUnconnection(pvaddr string, tp *TransportGroup, t *tox.Tox) *Unconnection {
	this := &Unconnection{}
	this.grptp = tp
	this.kcptp = NewKcpTransport2(t, pvaddr, false, tp)
	this.gip = pvaddr

	muxcfg := yamux.DefaultConfig()
	muxcfg.EnableKeepAlive = false
	// muxcfg.MaxStreamWindowSize = tunwndsz
	muxcli, err := yamux.Client(this, muxcfg)
	ErrPrint(err)
	this.muxcli = muxcli

	return this
}

func (this *Unconnection) Read(b []byte) (n int, err error) {
	log.Println(ldebugp, len(b))
	btime := time.Now()
	comevt := <-this.kcptp.getReadyReadChan()
	ub := comevt.v.Interface().([]byte)
	n = copy(b, ub)
	log.Println(ldebugp, len(b), n, time.Now().Sub(btime))
	// 198.655921ms
	// 可能是有问题的
	return
}

func (this *Unconnection) Write(b []byte) (n int, err error) {
	log.Println(ldebugp, len(b), this.tname, this.gip)
	this.kcptp.sendData(string(b), this.gip)
	return len(b), nil
}

func (this *Unconnection) Close() error {
	return nil
}

func init_check() {
	var c io.ReadWriteCloser = &Unconnection{}
	_ = c
}

////////////// server

type UnconnectionServer struct {
	tname  string
	gip    string
	grptp  *TransportGroup
	muxcli *yamux.Session
	kcptp  *KcpTransport

	stms  *hashmap.Map // stmid => *yamux.Stream
	_stms *yamux.Session
}

func newUnconnectionServer(pvaddr string, tp *TransportGroup, t *tox.Tox) *UnconnectionServer {
	this := &UnconnectionServer{}
	this.grptp = tp
	this.kcptp = NewKcpTransport2(t, pvaddr, true, tp)
	this.gip = pvaddr

	muxcfg := yamux.DefaultConfig()
	muxcfg.EnableKeepAlive = false
	// muxcfg.MaxStreamWindowSize = tunwndsz
	muxcli, err := yamux.Server(this, muxcfg)
	ErrPrint(err)
	this.muxcli = muxcli

	return this
}

func (this *UnconnectionServer) Read(b []byte) (n int, err error) {
	log.Println(ldebugp, len(b))
	comevt := <-this.kcptp.getReadyReadChan()
	ub := comevt.v.Interface().([]byte)
	n = copy(b, ub)
	log.Println(ldebugp, len(b), n, gopp.SubStr(string(b[:n]), 128))
	return
}

func (this *UnconnectionServer) Write(b []byte) (n int, err error) {
	log.Println(ldebugp, len(b), this.tname, this.gip, gopp.SubStr(string(b), 128))
	this.kcptp.sendData(string(b), this.gip)
	return len(b), nil
}

func (this *UnconnectionServer) Close() error {
	return nil
}

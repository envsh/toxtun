package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/url"
	"sync"

	// "log"
	"bytes"
	"encoding/binary"
	"net"
	"time"

	"github.com/envsh/go-toxcore/mintox"
	"github.com/kitech/goplusplus"
	"github.com/xtaci/smux"
)

type Tunneld struct {
	mtox *MTox

	mux1 *muxone

	kcpInputChan chan ClientReadyReadEvent
}

func NewTunneld() *Tunneld {
	this := new(Tunneld)

	this.mtox = newMinTox("toxtund")
	bcc, err := ioutil.ReadFile("./toxtunc.txt")
	debug.Println(err)
	bcc = bytes.TrimSpace(bcc)
	pubkey := mintox.CBDerivePubkey(mintox.NewCryptoKeyFromHex(string(bcc)))
	this.mtox.addFriend(pubkey.ToHex())
	log.Println("cli pubkey?", pubkey.ToHex())

	///
	this.init()
	return this
}

func (this *Tunneld) init() {
	this.kcpInputChan = make(chan ClientReadyReadEvent, mpcsz)

	// callbacks
	this.mtox.DataFunc = this.onMinToxData
	this.mtox.startup()
	for {
		select {
		case evt := <-this.kcpInputChan:
			this.processKcpInputChan(evt)
		}
	}
}

func (this *Tunneld) serve() {
	// like event handler
}

///////////

func (this *Tunneld) onKcpOutput2(buf []byte, size int, extra interface{}, prior bool) error {
	if size <= 0 {
		// 如果总是出现，并且不影响程序运行，那么也就不是bug了
		// info.Println("wtf")
		return fmt.Errorf("Invalid size %d", size)
	}
	debug.Println(len(buf), "//", size, "//", string(gopp.SubBytes(buf, 52)))
	// ch := extra.(*Channel)

	var sndlen int = size
	var err error
	err = this.mtox.sendData(buf[:size], false, prior)
	if err != nil {
		debug.Println(err)
	} else {
		debug.Println("kcp->tox:", sndlen, time.Now().String())
	}
	return err
}

func (this *Tunneld) onKcpOutput(buf []byte, size int, extra interface{}) {
	this.onKcpOutput2(buf, size, extra, false)
}

// should block
func (this *Tunneld) connectToBackend(stm *smux.Stream) {
	// Dial
	var conn net.Conn
	var err error
	uo, err := url.Parse(stm.Syndat())
	gopp.ErrPrint(err, stm.Syndat())

	// TODO 如果连接失败，响应的包会导致client崩溃
	if uo.Scheme == "tcp" {
		conn, err = net.Dial("tcp", uo.Host)
	} else if uo.Scheme == "udp" {
		conn, err = net.Dial("udp", uo.Host)
	} else {
		log.Panicln("not supported proto:", uo.Scheme)
	}

	if err != nil {
		errl.Println(err, stm.ID(), stm.Syndat())
		// 连接结束
		debug.Println("dial failed, cleaning up...:", stm.ID(), stm.Syndat())
		// ch.server_socket_close = true
		// this.serverCloseChan <- ServerCloseEvent{ch}
		stm.Close()
		appevt.Trigger("connact", -1)
		return
	}
	info.Println("connected to:", conn.RemoteAddr().String(), stm.ID(), stm.Syndat())
	// info.Println("channel connected,", ch.chidcli, ch.chidsrv, ch.conv, pkt.msgid)

	var wg sync.WaitGroup
	wg.Add(2)

	// srv -> net
	go func() {
		wn, err := io.Copy(stm, conn)
		gopp.ErrPrint(err, wn)
		wg.Done()
		stm.Close()
	}()

	// net -> srv
	go func() {
		wn, err := io.Copy(conn, stm)
		gopp.ErrPrint(err, wn)
		wg.Done()
		conn.Close()
	}()

	wg.Wait()
	stm.Close()
	conn.Close()
	log.Println("release conn", stm.ID(), stm.Syndat())
}

var spdc2 = mintox.NewSpeedCalc()

func (this *Tunneld) onMinToxData(data []byte, cbdata mintox.Object, ctrl bool) {
	this.kcpInputChan <- ClientReadyReadEvent{nil, data, len(data), ctrl}
}
func (this *Tunneld) processKcpInputChan(evt ClientReadyReadEvent) {
	data, ctrl := evt.buf, evt.ctrl
	message := string(data)
	friendId := this.mtox.friendpks
	if ctrl {
		pkt := parsePacket(bytes.NewBufferString(message).Bytes())
		this.handleCtrlPacket(pkt, friendId)
	} else {
		this.handleDataPacket(data, 0)
	}
}
func (this *Tunneld) handleDataPacket(buf []byte, friendNumber uint32) {
	// kcp包前4字段为conv，little hacky
	conv := binary.LittleEndian.Uint32(buf)
	if this.mux1 == nil {
		info.Println("mux1 conn not exist, drop pkt", conv)
		return
	}
	err := this.mux1.rudp_.Input(buf)
	gopp.ErrPrint(err)
	if err != nil {
		errl.Println("kcp input err:", conv, len(buf))
	} else {
		n := len(buf)
		debug.Println("tox->kcp:", conv, n, len(buf), gopp.StrSuf(string(buf), 52))
	}

}
func (this *Tunneld) handleCtrlPacket(pkt *Packet, friendId string) {
	if pkt.Command == CMDCONNSYN {
		info.Printf("New mux1 conn on tunnel %s to %s:%s:%s, conv: %d\n",
			pkt.Tunname, pkt.Tunproto, pkt.Remoteip, pkt.Remoteport, pkt.Conv)
		// close old, ack back, create new
		if this.mux1 != nil {
			mux1 := this.mux1
			this.mux1 = nil
			info.Println("mux1 conn exist, renew", mux1.conv, "->", pkt.Conv)
			mux1.Close()
			mux1.rudp_.Close()
		}

		repkt := &*pkt
		repkt.Command = CMDCONNACK
		err := this.mtox.sendData(repkt.toJson(), true, true)
		if err != nil {
			errl.Println(err)
		}

		writeout := func(data []byte, prior bool) error {
			return this.onKcpOutput2(data, len(data), nil, prior)
		}
		this.mux1 = NewMuxone(pkt.Conv, writeout)
		go this.acceptconn()

	} else if pkt.Command == CMDCLOSEFIN {
		log.Panicln("not used")
	} else {
		errl.Println("wtf, unknown cmmand:", pkt.Command, pkt.Chidcli, pkt.Chidsrv, pkt.Conv)
	}
}
func (this *Tunneld) acceptconn() {
	mux1 := this.mux1
	log.Println("mux1 accept loop start", mux1.conv, mux1.NumStreams())
	for {
		stm, err := mux1.AcceptStream()
		gopp.ErrPrint(err)
		if err != nil {
			break
		}
		log.Println("new stream conn", stm.ID(), stm.Syndat())
		go this.connectToBackend(stm)
	}
	log.Println("mux1 accept loop done", mux1.conv, mux1.NumStreams())
}

// a tool function
func (this *Tunneld) makeKcpConv(friendId string, pkt *Packet) uint32 {
	// crc32: toxid+host+port+time
	return makeKcpConv(friendId, pkt.Remoteip, pkt.Remoteport)
}

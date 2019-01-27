package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"math"
	"math/rand"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/envsh/go-toxcore/mintox"
	"github.com/kitech/goplusplus"
	deadlock "github.com/sasha-s/go-deadlock"
	"github.com/xtaci/smux"

	rudp "mkuse/rudp2"
)

var (
	// TODO dynamic multiple port mode
	// tunnel客户端通道监听服务端口
	tunnelServerPort = 8113
)

type TunListener struct {
	proto  string
	tcplsn net.Listener
	udplsn net.PacketConn
	fulsn  net.Listener
}

func newTunListenerUdp(lsn net.PacketConn) *TunListener {
	return &TunListener{proto: "udp", udplsn: lsn}
}
func newTunListenerUdp2(lsn net.Listener) *TunListener {
	return &TunListener{proto: "udp", fulsn: lsn}
}
func newTunListenerTcp(lsn net.Listener) *TunListener { return &TunListener{proto: "tcp", tcplsn: lsn} }

type Tunnelc struct {
	mtox *MTox
	srvs map[string]*TunListener

	mux1 *muxone

	newConnC      chan NewConnEvent
	muxConnedC    chan bool
	muxDisconnedC chan bool
	newconnq      []NewConnEvent // 连接阶段暂存连接请求
}

func NewTunnelc() *Tunnelc {
	this := new(Tunnelc)
	this.srvs = make(map[string]*TunListener)

	this.mtox = newMinTox("toxtunc")

	// callbacks
	this.mtox.DataFunc = this.onMinToxData
	this.mtox.startup()
	//
	return this
}

func (this *Tunnelc) serve() {
	// recs := config.recs
	this.listenTunnels()

	this.newConnC = make(chan NewConnEvent, mpcsz)
	this.muxConnedC = make(chan bool, 1)
	this.muxDisconnedC = make(chan bool, 1)

	this.serveTunnels()

	// like event handler
	for {
		select {
		case evt := <-this.newConnC:
			this.muxConnHandler("newconn", evt)
		case <-this.muxConnedC:
			this.muxConnHandler("muxconned", NewConnEvent{})
		case <-this.muxDisconnedC:
			this.muxConnHandler("muxdisconned", NewConnEvent{})
		}
	}
}

func (this *Tunnelc) listenTunnels() {
	for tname, tunrec := range config.recs {
		tunnelServerPort := tunrec.lport
		if tunrec.tproto == "tcp" {
			srv, err := net.Listen(tunrec.tproto, fmt.Sprintf(":%d", tunnelServerPort))
			if err != nil {
				log.Println(lerrorp, err)
				continue
			}
			this.srvs[tunrec.tname] = newTunListenerTcp(srv)
			this.mtox.addFriend(tunrec.rpubkey)
		} else if tunrec.tproto == "udp" {
			srv, err := ListenUDP(fmt.Sprintf(":%d", tunnelServerPort))
			if err != nil {
				log.Println(lerrorp, err, tname, tunnelServerPort)
				continue
			}
			this.srvs[tunrec.tname] = newTunListenerUdp2(srv)
			this.mtox.addFriend(tunrec.rpubkey)
		} else {
			log.Panicln("wtf,", tunrec)
		}
		log.Println(linfop, fmt.Sprintf("#T%s", tname), "tunaddr:", tunrec.tname, tunrec.tproto, tunrec.lhost, tunrec.lport)
	}
}

func (this *Tunnelc) serveTunnels() {
	srvs := this.srvs
	for tname, srv := range srvs {
		if srv.proto == "tcp" {
			go this.serveTunnel(tname, srv.tcplsn)
		} else if srv.proto == "udp" {
			go this.serveTunnelUdp(tname, srv.fulsn)
			// go this.serveTunnelUdp(tname, srv.udplsn)
		}
	}
}

// should blcok
func (this *Tunnelc) serveTunnel(tname string, srv net.Listener) {
	info.Println("serving tcp:", tname, srv.Addr())
	for {
		c, err := srv.Accept()
		if err != nil {
			info.Println(err)
		}
		// info.Println(c)
		info.Println("New conn :", c.RemoteAddr(), "->", c.LocalAddr(), tname)
		this.newConnC <- NewConnEvent{c, 0, time.Now(), tname}
		appevt.Trigger("newconn", tname)
	}
}

// should blcok
func (this *Tunnelc) serveTunnelUdp(tname string, srv net.Listener) {
	info.Println("serving udp:", tname, srv.Addr())
	for {
		c, err := srv.Accept()
		if err != nil {
			info.Println(err)
		}
		// info.Println(c)
		info.Println("New conn :", c.RemoteAddr(), "->", c.LocalAddr(), tname)
		this.newConnC <- NewConnEvent{c, 0, time.Now(), tname}
		appevt.Trigger("newconn", tname)
	}
}

var convid uint32 = rand.Uint32() % (math.MaxUint32 / 8)
var mux1mu deadlock.RWMutex
var mux1C = make(chan bool)

func nextconvid() uint32 {
	return atomic.AddUint32(&convid, 1)
}

// should not block
func (this *Tunnelc) muxConnHandler(evtname string, evt NewConnEvent) {
	switch evtname {
	case "newconn":
		if this.mux1 == nil || (this.mux1 != nil && this.mux1.IsClosed()) {
			mux1 := this.mux1
			this.mux1 = nil
			if mux1 != nil {
				mux1.Close()
				logger.Infoln("mux1 conn is closed, reconnect ...", mux1.conv)
			}

			this.newconnq = append(this.newconnq, evt)
			go this.connectmux1(evt.tname)
		} else {
			// server conn
			go this.serveconnevt(this.mux1, evt)
		}

	case "muxconned":
		info.Println("muxconned")
		for _, evt_ := range this.newconnq {
			evt := evt_
			// server conn
			go this.serveconnevt(this.mux1, evt)
		}
		this.newconnq = nil
	case "muxdisconned":
	default:
		log.Panicln("unknown evtname", evtname)
	}
}

func (this *Tunnelc) connectmux1(tname string) error {
	for retry := 0; retry < 3; retry++ {
		err := this.connectmux1impl(tname, retry)
		if err == nil {
			break
		}
		time.Sleep(5 * time.Second)
	}
	return nil
}
func (this *Tunnelc) connectmux1impl(tname string, retry int) error {
	info.Println("mux conning ...", retry)
	tunrec := config.getRecordByName(tname)
	conv := nextconvid()
	pkt := NewBrokenPacket(conv)
	pkt.Command = CMDCONNSYN
	pkt.Tunname = tunrec.tname
	pkt.Remoteip = tunrec.rhost
	pkt.Remoteport = gopp.ToStr(tunrec.rport)
	pktdat := pkt.toJson()
	pba := rudp.NewPfxByteArray(len(pktdat))
	pba.Copy(pktdat)
	err := this.mtox.sendData(pba, true, true)
	gopp.ErrPrint(err, conv)

	info.Println("wait conned ...", retry)
	select {
	case <-mux1C:
	case <-time.After(10 * time.Second):
		err = fmt.Errorf("mux1 conn timeout %d", conv)
		log.Println("close cli conn", retry, err)
		return err
	}

	writeout := func(data *rudp.PfxByteArray, prior bool) error {
		return this.onKcpOutput2(data, nil, prior)
	}
	this.mux1 = NewMuxone(conv, writeout)
	this.muxConnedC <- true
	return nil
}
func (this *Tunnelc) serveconnevt(mux1 *muxone, evt NewConnEvent) {
	// server conn
	conn := evt.conn
	times := evt.times
	btime := evt.btime
	tname := evt.tname
	this.serveconn(this.mux1, conn, times, btime, tname)
}
func (this *Tunnelc) serveconn(mux1 *muxone, conn net.Conn, times int, btime time.Time, tname string) {
	info.Println("serving conn ...", tname)
	tunrec := config.getRecordByName(tname)
	btime2 := time.Now()
	syndat := fmt.Sprintf("%s://%s:%d/%s", tunrec.tproto, tunrec.rhost, tunrec.rport, tunrec.tname)

	var stm *smux.Stream
	var err error
	stmC := make(chan bool, 0)
	go func() {
		stm_, err_ := mux1.OpenStream(syndat)
		if err == nil {
			select {
			case stmC <- true:
				stm = stm_
				err = err_
			default:
				stm_.Close()
			}
		}
	}()
	select {
	case <-stmC:
	case <-time.After(5 * time.Second):
		err = fmt.Errorf("open steam timeout")
	}
	gopp.ErrPrint(err)
	if err != nil {
		if times >= 5 {
			log.Println("stm conn failed timeout", times, conn.RemoteAddr(), mux1.IsClosed(), err)
			conn.Close()
			mux1.Close()
			return
		}
		log.Println("stm conn failed, retry", times, conn.RemoteAddr(), mux1.IsClosed(), err)
		time.Sleep(3 * time.Second)
		this.newConnC <- NewConnEvent{conn, times + 1, btime, tname}
		return
	}
	log.Println("stm open time", stm.ID(), time.Since(btime2))

	var wg sync.WaitGroup
	wg.Add(2)

	// cli -> net
	go func() {
		wn, err := io.Copy(stm, conn)
		gopp.ErrPrint(err, stm.ID(), wn)
		info.Println("conn cli -> net done ", stm.ID())
		wg.Done()
		stm.Close()
		/*
			2019/01/23 21:13:05.354989 tunnelc.go:236: read tcp 127.0.0.1:8113->127.0.0.1:50844: use of closed network connection 8 13987221
			2019/01/23 21:13:05.355019 tunnelc.go:252: done cli conn 8 127.0.0.1:50844
		*/
	}()

	// net -> cli
	go func() {
		wn, err := io.Copy(conn, stm)
		gopp.ErrPrint(err, stm.ID(), wn)
		info.Println("conn net -> cli done ", stm.ID())
		wg.Done()

		select {
		case <-time.After(10 * time.Second):
			conn.Close()
		}
	}()

	wg.Wait()
	conn.Close()
	stm.Close()
	log.Println("done cli conn", stm.ID(), conn.RemoteAddr())
	return
}

func (this *Tunnelc) onKcpOutput2(buf *rudp.PfxByteArray, extra interface{}, prior bool) error {
	size := buf.FullLen()
	if size <= 0 {
		// 如果总是出现，并且不影响程序运行，那么也就不是bug了
		return fmt.Errorf("invalid size %d", size)
	}

	// tunrec := config.getRecordByName(ch.tname)
	// toxtunid := tunrec.rpubkey

	var sndlen int = size
	var err error
	err = this.mtox.sendData(buf, false, prior)
	if err != nil {
		debug.Println(err)
	} else {
		debug.Println("kcp->tox:", sndlen)
	}
	return err
}

func (this *Tunnelc) onKcpOutput(buf *rudp.PfxByteArray, extra interface{}) {
	this.onKcpOutput2(buf, extra, false)
}

// TODO 应该把数据发送到chan进入主循环再回来处理，就不会有concurrent问题
func (this *Tunnelc) onMinToxData(data []byte, cbdata mintox.Object, ctrl bool) {
	debug.Println(len(data), ctrl)

	if ctrl {
		message := string(data)
		pkt := parsePacket(bytes.NewBufferString(message).Bytes())
		this.handleCtrlPacket(pkt, 0)
	} else {
		this.handleDataPacket(data, 0)
	}
}
func (this *Tunnelc) handleCtrlPacket(pkt *Packet, friendNumber uint32) {
	if pkt.Command == CMDCONNACK {
		info.Println("channel connected,", pkt.Conv, pkt.Data)
		select {
		case mux1C <- true:
		case <-time.After(5 * time.Second):
			log.Panicln("wtf", pkt.Conv, pkt.Data)
		}
	} else if pkt.Command == CMDCLOSEFIN {
	} else {
		errl.Println("wtf, unknown cmmand:", pkt.Command, pkt.Chidcli, pkt.Chidsrv, pkt.Conv)
	}
}

func (this *Tunnelc) handleDataPacket(buf []byte, friendNumber uint32) {
	var conv uint32
	// kcp包前4字段为conv，little hacky
	if len(buf) < 4 {
		errl.Println("wtf")
	}
	conv = binary.LittleEndian.Uint32(buf)

	if this.mux1 == nil {
		info.Println("mux1 conn not exist, drop pkt", conv)
		return
	}
	this.mux1.rudp_.Input(buf)
	n := len(buf)
	debug.Println("tox->kcp:", conv, n, len(buf), gopp.StrSuf(string(buf), 52))
}

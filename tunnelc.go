package main

import (
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"math"
	"math/rand"
	"net"
	"sync"
	"sync/atomic"

	// "strings"
	"bytes"
	"time"

	"github.com/envsh/go-toxcore/mintox"
	"github.com/kitech/goplusplus"
	deadlock "github.com/sasha-s/go-deadlock"
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

	newConnChan chan NewConnEvent
}

func NewTunnelc() *Tunnelc {
	this := new(Tunnelc)
	this.srvs = make(map[string]*TunListener)

	this.mtox = newMinTox("toxtunc")

	// callbacks
	this.mtox.DataFunc = this.onMinToxData
	//
	return this
}

func (this *Tunnelc) serve() {
	// recs := config.recs
	this.listenTunnels()

	this.newConnChan = make(chan NewConnEvent, mpcsz)

	this.serveTunnels()

	// like event handler
	for {
		select {
		case evt := <-this.newConnChan:
			this.initConnChannel(evt.conn, evt.times, evt.btime, evt.tname)
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
		info.Println("New connection from/to:", c.RemoteAddr(), c.LocalAddr(), tname)
		this.newConnChan <- NewConnEvent{c, 0, time.Now(), tname}
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
		info.Println("New connection from/to:", c.RemoteAddr(), c.LocalAddr(), tname)
		this.newConnChan <- NewConnEvent{c, 0, time.Now(), tname}
		appevt.Trigger("newconn", tname)
	}
}

var convid uint32 = rand.Uint32() % (math.MaxUint32 / 8)
var mux1mu deadlock.RWMutex
var mux1C = make(chan bool)

func nextconvid() uint32 {
	return atomic.AddUint32(&convid, 1)
}

func (this *Tunnelc) initConnChannel(conn net.Conn, times int, btime time.Time, tname string) error {
	err := this.connectmux1(tname)
	gopp.ErrPrint(err)
	if err != nil {
		conn.Close()
		return err
	}

	// server conn
	go this.serveconn(this.mux1, conn, times, btime, tname)
	return nil
}
func (this *Tunnelc) connectmux1(tname string) error {
	if this.mux1 != nil {
		if this.mux1.IsClosed() {
			mux1 := this.mux1
			this.mux1 = nil
			mux1.rudp_.Close()
			logger.Infoln("mux1 conn is closed, reconnect ...", mux1.conv)
		} else {
			return nil
		}
	}
	tunrec := config.getRecordByName(tname)
	conv := nextconvid()
	pkt := NewBrokenPacket(conv)
	pkt.Command = CMDCONNSYN
	pkt.Tunname = tunrec.tname
	pkt.Remoteip = tunrec.rhost
	pkt.Remoteport = gopp.ToStr(tunrec.rport)
	err := this.mtox.sendData(pkt.toJson(), true, true)
	gopp.ErrPrint(err, conv)

	select {
	case <-mux1C:
	case <-time.After(15 * time.Second):
		err = fmt.Errorf("conn mux1 timeout %d", conv)
		log.Println("close cli conn", err)
		return err
	}

	writeout := func(data []byte, prior bool) error {
		return this.onKcpOutput2(data, len(data), nil, prior)
	}
	this.mux1 = NewMuxone(conv, writeout)
	return nil
}
func (this *Tunnelc) serveconn(mux1 *muxone, conn net.Conn, times int, btime time.Time, tname string) {

	tunrec := config.getRecordByName(tname)
	btime2 := time.Now()
	syndat := fmt.Sprintf("%s://%s:%d/%s", tunrec.tproto, tunrec.rhost, tunrec.rport, tunrec.tname)
	stm, err := mux1.OpenStream(syndat)
	gopp.ErrPrint(err)
	if err != nil {
		if times > 5 {
			log.Println("stm conn failed timeout", times, conn.RemoteAddr(), mux1.IsClosed(), err)
			conn.Close()
			return
		}
		log.Println("stm conn failed, retry", times, conn.RemoteAddr(), mux1.IsClosed(), err)
		time.Sleep(3 * time.Second)
		this.newConnChan <- NewConnEvent{conn, times + 1, btime, tname}
		return
	}
	log.Println("stm open time", time.Since(btime2))

	var wg sync.WaitGroup
	wg.Add(2)

	// cli -> net
	go func() {
		wn, err := io.Copy(stm, conn)
		gopp.ErrPrint(err, stm.ID(), wn)
		wg.Done()
		stm.Close()
	}()

	// net -> cli
	go func() {
		wn, err := io.Copy(conn, stm)
		gopp.ErrPrint(err, stm.ID(), wn)
		wg.Done()
		conn.Close()
	}()

	wg.Wait()
	conn.Close()
	stm.Close()
	log.Println("done cli conn", stm.ID(), conn.RemoteAddr())
	return
}

func (this *Tunnelc) onKcpOutput2(buf []byte, size int, extra interface{}, prior bool) error {
	if size <= 0 {
		// 如果总是出现，并且不影响程序运行，那么也就不是bug了
		return fmt.Errorf("invalid size %d", size)
	}

	// tunrec := config.getRecordByName(ch.tname)
	// toxtunid := tunrec.rpubkey

	var sndlen int = size
	var err error
	err = this.mtox.sendData(buf[:size], false, prior)
	if err != nil {
		debug.Println(err)
	} else {
		debug.Println("kcp->tox:", sndlen)
	}
	return err
}

func (this *Tunnelc) onKcpOutput(buf []byte, size int, extra interface{}) {
	this.onKcpOutput2(buf, size, extra, false)
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
		info.Println("mux1 conn not exist, drop pkt", conv, convid)
		return
	}
	this.mux1.rudp_.Input(buf)
	n := len(buf)
	debug.Println("tox->kcp:", conv, n, len(buf), gopp.StrSuf(string(buf), 52))
}

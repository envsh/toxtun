package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"math"
	"math/rand"
	"net"
	"sync"
	"sync/atomic"
	"time"

	tox "github.com/TokTok/go-toxcore-c"
	"github.com/kitech/goplusplus"
	deadlock "github.com/sasha-s/go-deadlock"

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
	tox *tox.Tox

	// mtox *MTox
	srvs map[string]*TunListener

	// mux1    *muxone
	mtvtp   *rudp.Vtpconn
	muxsess MuxSession

	newConnC      chan NewConnEvent
	muxConnedC    chan bool
	muxDisconnedC chan bool
	newconnq      []NewConnEvent // 连接阶段暂存连接请求
	toxPollChan   chan ToxPollEvent
}

func NewTunnelc() *Tunnelc {
	this := new(Tunnelc)
	this.srvs = make(map[string]*TunListener)

	t := makeTox("toxtunc")
	this.tox = t
	recs := config.recs
	this.tox.SelfSetStatusMessage(fmt.Sprintf("%s of toxtun, %+v", "toxtuncs", recs))

	// callbacks
	t.CallbackSelfConnectionStatus(this.onToxnetSelfConnectionStatus, nil)
	t.CallbackFriendRequest(this.onToxnetFriendRequest, nil)
	t.CallbackFriendConnectionStatus(this.onToxnetFriendConnectionStatus, nil)
	t.CallbackFriendMessage(this.onToxnetFriendMessage, nil)
	t.CallbackFriendLossyPacket(this.onToxnetFriendLossyPacket, nil)
	t.CallbackFriendLosslessPacket(this.onToxnetFriendLosslessPacket, nil)

	// this.mtox = newMinTox("toxtunc")
	// callbacks
	// this.mtox.DataFunc = this.onMinToxData
	// this.mtox.startup()

	writeout := this.writeout4tox
	this.mtvtp = rudp.NewVtpconn(writeout)
	//
	return this
}

func (this *Tunnelc) writeout4tox(buf []byte) error {
	data := rudp.PfxBuffp().Get()
	data.Copy(buf)
	defer rudp.PfxBuffp().Put(data)
	prior := false
	return this.onKcpOutput2(data, nil, prior)
}

func (this *Tunnelc) writeout4mtox(buf []byte) error {
	data := rudp.PfxBuffp().Get()
	data.Copy(buf)
	defer rudp.PfxBuffp().Put(data)
	prior := false
	return this.onKcpOutput2(data, nil, prior)
}

func (this *Tunnelc) serve() {
	// recs := config.recs
	this.listenTunnels()

	this.newConnC = make(chan NewConnEvent, mpcsz)
	this.muxConnedC = make(chan bool, 1)
	this.muxDisconnedC = make(chan bool, 1)
	this.toxPollChan = make(chan ToxPollEvent, mpcsz)

	this.serveTunnels()

	// install pollers
	go func() {
		for {
			time.Sleep(time.Duration(smuse.tox_interval) * time.Millisecond)
			// time.Sleep(200 * time.Millisecond)
			// this.toxPollChan <- ToxPollEvent{}
			iterate(this.tox)
		}
	}()

	// like event handler
	for {
		select {
		case evt := <-this.newConnC:
			this.muxConnHandler("newconn", evt)
		case <-this.muxConnedC:
			this.muxConnHandler("muxconned", NewConnEvent{})
		case <-this.muxDisconnedC:
			this.muxConnHandler("muxdisconned", NewConnEvent{})
		case <-this.toxPollChan:

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
			// this.mtox.addFriend(tunrec.rpubkey)
			_, err = this.tox.FriendAdd(tunrec.rpubkey, "hiyo")
			gopp.ErrPrint(err, tunrec.rpubkey)
		} else if tunrec.tproto == "udp" {
			srv, err := ListenUDP(fmt.Sprintf(":%d", tunnelServerPort))
			if err != nil {
				log.Println(lerrorp, err, tname, tunnelServerPort)
				continue
			}
			this.srvs[tunrec.tname] = newTunListenerUdp2(srv)
			// this.mtox.addFriend(tunrec.rpubkey)
			_, err = this.tox.FriendAdd(tunrec.rpubkey, "hiyo")
			gopp.ErrPrint(err, tunrec.rpubkey)
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
		muxsess := this.muxsess
		if muxsess == nil || (muxsess != nil && muxsess.IsClosed()) {
			this.muxsess = nil
			if muxsess != nil {
				muxsess.Close()
				logger.Infoln("mux1 conn is closed, reconnect ...", muxsess.SessID())
			}

			this.newconnq = append(this.newconnq, evt)
			go this.connectmux1(evt.tname)
		} else {
			// server conn
			go this.serveconnevt(this.muxsess, evt)
		}

	case "muxconned":
		info.Println("muxconned")
		for _, evt_ := range this.newconnq {
			evt := evt_
			// server conn
			go this.serveconnevt(this.muxsess, evt)
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
	/*
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
	*/

	/*
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
	*/

	addr := rudp.NewVtpAddr(fmt.Sprintf("toxrly.%s.%s", tunrec.tproto, tunrec.tname),
		fmt.Sprintf("%s:%v", tunrec.rhost, tunrec.rport))
	muxsess, err := RudpDialSync(this.mtvtp, addr)
	gopp.ErrPrint(err)
	if err != nil {
		return err
	}

	this.muxsess = muxsess
	this.muxConnedC <- true
	return nil
}
func (this *Tunnelc) serveconnevt(muxsess MuxSession, evt NewConnEvent) {
	// server conn
	conn := evt.conn
	times := evt.times
	btime := evt.btime
	tname := evt.tname
	this.serveconn(muxsess, conn, times, btime, tname)
}
func (this *Tunnelc) serveconn(mux1 MuxSession, conn net.Conn, times int, btime time.Time, tname string) {
	info.Println("serving conn ...", tname)
	tunrec := config.getRecordByName(tname)
	btime2 := time.Now()
	syndat := fmt.Sprintf("%s://%s:%d/%s", tunrec.tproto, tunrec.rhost, tunrec.rport, tunrec.tname)

	var stm MuxStream
	var err error
	stmC := make(chan bool, 0)
	go func() {
		stm_, err_ := mux1.OpenStream(syndat)
		if err_ == nil {
			stm = stm_
			err = err_
			select {
			case stmC <- true:
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
	log.Println("stm open time", stm.StreamID(), time.Since(btime2))

	var wg sync.WaitGroup
	wg.Add(2)

	// cli -> net
	go func() {
		wn, err := io.Copy(stm, conn)
		gopp.ErrPrint(err, stm.StreamID(), wn)
		info.Println("conn cli -> net done ", stm.StreamID())
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
		gopp.ErrPrint(err, stm.StreamID(), wn)
		info.Println("conn net -> cli done ", stm.StreamID())
		wg.Done()

		select {
		case <-time.After(10 * time.Second):
			conn.Close()
		}
	}()

	wg.Wait()
	conn.Close()
	stm.Close()
	log.Println("done cli conn", stm.StreamID(), conn.RemoteAddr())
	return
}

func (this *Tunnelc) onKcpOutput2(buf *rudp.PfxByteArray, extra interface{}, prior bool) error {
	size := buf.FullLen()
	if size <= 0 {
		// 如果总是出现，并且不影响程序运行，那么也就不是bug了
		return fmt.Errorf("invalid size %d", size)
	}

	// tunrec := config.getRecordByName(ch.tname)
	var toxtunid string
	for _, reco := range config.recs {
		toxtunid = reco.rpubkey
		break
	}
	fnum, err := this.tox.FriendByPublicKey(toxtunid)
	gopp.ErrPrint(err, toxtunid)
	if err != nil {
		return err
	}

	var sndlen int = size
	// var err error
	// err = this.mtox.sendData(buf, false, prior)
	buf.PPU8(254)
	err = this.tox.FriendSendLossyPacket(fnum, string(buf.FullData()))
	gopp.ErrPrint(err, buf.RawLen())
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
func (this *Tunnelc) onMinToxData(data []byte) {
	debug.Println(len(data))
	err := this.mtvtp.Input(data)
	gopp.ErrPrint(err)

	/*
		if ctrl {
			message := string(data)
			pkt := parsePacket(bytes.NewBufferString(message).Bytes())
			this.handleCtrlPacket(pkt, 0)
		} else {
			this.handleDataPacket(data, 0)
		}
	*/
}

/*
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
*/

//////////////
func (this *Tunnelc) onToxnetSelfConnectionStatus(t *tox.Tox, status int, extra interface{}) {
	info.Println("mytox status:", status)
	for tname, _ := range this.srvs {
		tunrec := config.getRecordByName(tname)
		this.onToxnetSelfConnectionStatusImpl(t, status, extra, tunrec.tname, tunrec.rpubkey)
	}
	if status == 0 {
		switchServer(t)
	} else {
		addLiveBots(t)
		t.WriteSavedata(tox_savedata_fname)
	}

	if status == 0 {
		appevt.Trigger("selfonline", false)
		appevt.Trigger("selfoffline")
	} else {
		appevt.Trigger("selfonline", true)
	}
}

// 尝试添加为好友
func (this *Tunnelc) onToxnetSelfConnectionStatusImpl(t *tox.Tox, status int, extra interface{},
	tname string, toxtunid string) {
	friendNumber, err := t.FriendByPublicKey(toxtunid)
	log.Println(friendNumber, err, len(toxtunid), toxtunid)
	if err == nil {
		if false {
			t.FriendDelete(friendNumber)
		}
	}
	if err != nil {
		t.FriendAdd(toxtunid, fmt.Sprintf("hello, i'am tuncli of %s", tname))
		t.WriteSavedata(tox_savedata_fname)
	}
}

func (this *Tunnelc) onToxnetFriendRequest(t *tox.Tox, friendId string, message string, userData interface{}) {
	debug.Println(friendId, message)
}

func (this *Tunnelc) onToxnetFriendConnectionStatus(t *tox.Tox, friendNumber uint32, status int, userData interface{}) {
	fid, _ := this.tox.FriendGetPublicKey(friendNumber)
	info.Println("peer status (fn/st/id):", friendNumber, status, fid)
	if status == 0 {
		if friendInConfig(fid) {
			switchServer(t)
		}
	}

	livebotsOnFriendConnectionStatus(t, friendNumber, status)

	if status == 0 {
		appevt.Trigger("peeronline", false)
		appevt.Trigger("peeroffline")
	} else {
		appevt.Trigger("peeronline", true)
	}
}

func (this *Tunnelc) onToxnetFriendMessage(t *tox.Tox, friendNumber uint32, message string, userData interface{}) {
	debug.Println(friendNumber, len(message), gopp.StrSuf(message, 52))
	pkt := parsePacket(bytes.NewBufferString(message).Bytes())
	if pkt == nil {
		info.Println("maybe not command, just normal message:", gopp.StrSuf(message, 52))
	} else {
	}
}

func (this *Tunnelc) onToxnetFriendLossyPacket(t *tox.Tox, friendNumber uint32, message string, userData interface{}) {
	debug.Println(friendNumber, len(message), gopp.StrSuf(message, 52))
	buf := bytes.NewBufferString(message).Bytes()
	if buf[0] == 254 { // lossypacket
		data := buf[1:]
		debug.Println(len(data))
		err := this.mtvtp.Input(data)
		gopp.ErrPrint(err)
	} else {
		info.Println("unknown message:", buf[0])
	}
}

func (this *Tunnelc) onToxnetFriendLosslessPacket(t *tox.Tox, friendNumber uint32, message string, userData interface{}) {
	debug.Println(friendNumber, len(message), gopp.StrSuf(message, 52))
	buf := bytes.NewBufferString(message).Bytes()
	if buf[0] == 191 { // lossypacket
		buf = buf[1:]
		/*
			var conv uint32
			// kcp包前4字节为conv，little hacky
			if len(buf) < 4 {
				errl.Println("wtf")
			}

			conv = binary.LittleEndian.Uint32(buf)
			ch := this.chpool.pool2[conv]
			if ch == nil {
				errl.Println("maybe has some problem")
			}
			n := ch.kcp.Input(buf, true, true)
			debug.Println("tox->kcp:", conv, n, len(buf), gopp.StrSuf(string(buf), 52))
		*/
	} else {
		info.Println("unknown message:", buf[0])
	}
}

func (this *Tunnelc) FriendSendMessage(friendId string, message string) (uint32, error) {
	friendNumber, err := this.tox.FriendByPublicKey(friendId)
	if err != nil {
		return 0, err
	}
	return this.tox.FriendSendMessage(friendNumber, message)
}

func (this *Tunnelc) FriendSendLossyPacket(friendId string, message string) error {
	friendNumber, err := this.tox.FriendByPublicKey(friendId)
	if err != nil {
		return err
	}
	return this.tox.FriendSendLossyPacket(friendNumber, message)
}

package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"mkuse/rudp"
	"sync/atomic"

	// "log"
	"bytes"
	"encoding/binary"
	"net"
	"time"

	tox "github.com/TokTok/go-toxcore-c"
	"github.com/envsh/go-toxcore/mintox"
	"github.com/kitech/goplusplus"
	"golang.org/x/time/rate"
)

type Tunneld struct {
	tox     *tox.Tox
	mtox    *MTox
	usemtox bool
	chpool  *ChannelPool

	kcpNextUpdateWait int32

	toxPollChan chan ToxPollEvent
	// toxReadyReadChan    chan ToxReadyReadEvent
	// toxMessageChan      chan ToxMessageEvent
	kcpPollChan       chan KcpPollEvent
	kcpCheckCloseChan chan KcpCheckCloseEvent
	// kcpReadyReadChan    chan KcpReadyReadEvent
	// kcpOutputChan       chan KcpOutputEvent
	kcpInputChan        chan ClientReadyReadEvent
	serverReadyReadChan chan ServerReadyReadEvent
	serverCloseChan     chan ServerCloseEvent
	channelGCChan       chan ChannelGCEvent
}

func NewTunneld() *Tunneld {
	this := new(Tunneld)
	this.chpool = NewChannelPool()

	t := makeTox("toxtund")
	this.tox = t

	this.mtox = newMinTox("toxtund")
	bcc, err := ioutil.ReadFile("./toxtunc.txt")
	debug.Println(err)
	bcc = bytes.TrimSpace(bcc)
	pubkey := mintox.CBDerivePubkey(mintox.NewCryptoKeyFromHex(string(bcc)))
	this.mtox.addFriend(pubkey.ToHex())
	log.Println("cli pubkey?", pubkey.ToHex())
	this.usemtox = true

	///
	this.init()
	return this
}

func (this *Tunneld) init() {
	this.toxPollChan = make(chan ToxPollEvent, mpcsz)
	// this.toxReadyReadChan = make(chan ToxReadyReadEvent, 0)
	// this.toxMessageChan = make(chan ToxMessageEvent, 0)
	this.kcpPollChan = make(chan KcpPollEvent, mpcsz)
	this.kcpCheckCloseChan = make(chan KcpCheckCloseEvent, mpcsz)
	// this.kcpReadyReadChan = make(chan KcpReadyReadEvent, 0)
	// this.kcpOutputChan = make(chan KcpOutputEvent, 0)
	this.kcpInputChan = make(chan ClientReadyReadEvent, mpcsz)
	this.channelGCChan = make(chan ChannelGCEvent, mpcsz)
	this.serverReadyReadChan = make(chan ServerReadyReadEvent, mpcsz)
	this.serverCloseChan = make(chan ServerCloseEvent, mpcsz)

	// callbacks
	this.mtox.DataFunc = this.onMinToxData

	// callbacks
	t := this.tox
	t.CallbackSelfConnectionStatus(this.onToxnetSelfConnectionStatus, nil)
	t.CallbackFriendRequest(this.onToxnetFriendRequest, nil)
	t.CallbackFriendConnectionStatus(this.onToxnetFriendConnectionStatus, nil)
	t.CallbackFriendMessage(this.onToxnetFriendMessage, nil)
	t.CallbackFriendLossyPacket(this.onToxnetFriendLossyPacket, nil)
	t.CallbackFriendLosslessPacket(this.onToxnetFriendLosslessPacket, nil)

}

func (this *Tunneld) serve() {
	// install pollers
	go func() {
		for {
			time.Sleep(time.Duration(smuse.tox_interval) * time.Millisecond)
			// time.Sleep(30 * time.Millisecond)
			this.toxPollChan <- ToxPollEvent{}
		}
	}()
	go func() {
		for {
			if atomic.LoadInt32(&this.kcpNextUpdateWait) > 0 {
				time.Sleep(time.Duration(atomic.LoadInt32(&this.kcpNextUpdateWait)) * time.Millisecond)
			} else {
				// time.Sleep(20 * time.Millisecond)
				time.Sleep(time.Duration(smuse.kcp_interval) * time.Millisecond)
			}
			this.kcpPollChan <- KcpPollEvent{}
		}
	}()

	go func() {
		for {
			time.Sleep(1000 * time.Millisecond)
			this.kcpCheckCloseChan <- KcpCheckCloseEvent{}
		}
	}()

	go func() {
		for {
			time.Sleep(15 * time.Second)
			this.channelGCChan <- ChannelGCEvent{}
		}
	}()

	// like event handler
	for {
		select {
		// case evt := <-this.kcpReadyReadChan:
		//	this.processKcpReadyRead(evt.ch)
		// case evt := <-this.kcpOutputChan:
		//	this.processKcpOutput(evt.buf, evt.size, evt.extra)
		case evt := <-this.kcpInputChan:
			// evt.ch.kcp.Input(evt.buf, true, true)
			this.processKcpInputChan(evt)
		case evt := <-this.serverReadyReadChan:
			this.processServerReadyRead(evt.ch, evt.buf, evt.size)
		case evt := <-this.serverCloseChan:
			this.promiseChannelClose(evt.ch)
		case <-this.toxPollChan:
			if !this.usemtox {
				iterate(this.tox)
			}
			// case evt := <-this.toxReadyReadChan:
			// 	this.processFriendLossyPacket(this.tox, evt.friendNumber, evt.message, nil)
			// case evt := <-this.toxMessageChan:
			// 	debug.Println(evt)
			// 	this.processFriendMessage(this.tox, evt.friendNumber, evt.message, nil)
		case <-this.kcpPollChan:
			this.serveKcp()
		case <-this.kcpCheckCloseChan:
			this.kcpCheckClose()
		case <-this.channelGCChan:
			this.channelGC()
		}
	}
}

///////////
// TODO 计算kcpNextUpdateWait的逻辑优化
func kcp_poll(pool map[int32]*Channel) (chks []*Channel, nxtss []uint32) {
	for _, ch := range pool { // TODO fatal error: concurrent map iteration and map write
		if ch.kcp == nil {
			continue
		}
		curts := uint32(iclock())
		rts := ch.kcp.Check()
		if rts == curts {
			nxtss = append(nxtss, 10)
			chks = append(chks, ch)
		} else {
			nxtss = append(nxtss, rts-curts)
		}
	}

	return
}

func (this *Tunneld) serveKcp() {
	zbuf := make([]byte, 0)
	if true {
		chks, nxtss := kcp_poll(this.chpool.pool)

		mints := gopp.MinU32(nxtss)
		if mints > 10 && mints != math.MaxUint32 {
			// this.kcpNextUpdateWait = int(mints)
			atomic.StoreInt32(&this.kcpNextUpdateWait, int32(mints))
			return
		} else {
			// this.kcpNextUpdateWait = 10
			atomic.StoreInt32(&this.kcpNextUpdateWait, 10)
		}

		for _, ch := range chks {
			if ch.kcp == nil {
				continue
			}

			ch.kcp.Update()

			n := ch.kcp.Recv(zbuf)
			switch n {
			case -3:
				this.processKcpReadyRead(ch)
			case -2: // just empty kcp recv queue
				// errl.Println("kcp recv internal error:", n, this.kcp.PeekSize())
			case -1: // EAGAIN
			default:
				errl.Println("unknown recv:", n)
			}
		}
	}
}
func (this *Tunneld) kcpCheckClose() {
	closed := make([]*Channel, 0)
	for _, ch := range this.chpool.pool {
		if ch.kcp == nil {
			continue
		}

		if ch.server_socket_close == true && ch.server_kcp_close == false {
			cnt := ch.kcp.WaitSnd()
			if cnt == 0 {
				debug.Println("channel empty:", ch.chidcli, ch.chidsrv, ch.conv)
				ch.server_kcp_close = true
				closed = append(closed, ch)
			}
		}
	}

	for _, ch := range closed {
		this.promiseChannelClose(ch)
	}
}

func (this *Tunneld) onKcpOutput2(buf []byte, size int, extra interface{}, prior bool) error {
	if size <= 0 {
		// 如果总是出现，并且不影响程序运行，那么也就不是bug了
		// info.Println("wtf")
		return fmt.Errorf("Invalid size %d", size)
	}
	debug.Println(len(buf), "//", size, "//", string(gopp.SubBytes(buf, 52)))
	ch := extra.(*Channel)

	var sndlen int = size
	var err error
	if !this.usemtox {
		sndlen += 1
		msg := string([]byte{254}) + string(buf[:size])
		err = this.FriendSendLossyPacket(ch.toxid, msg)
		// msg := string([]byte{191}) + string(buf[:size])
		// err := this.tox.FriendSendLosslessPacket(0, msg)
	} else {
		err = this.mtox.sendData(buf[:size], false, prior)
	}
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

func (this *Tunneld) processKcpReadyRead(ch *Channel) {
	if ch.conn == nil {
		errl.Println("Not Connected:", ch.chidsrv, ch.chidcli, ch.tname)
		return
	}

	buf := make([]byte, ch.kcp.PeekSize())
	n := ch.kcp.Recv(buf)

	if len(buf) != n {
		errl.Println("Invalide kcp recv data", ch.tname)
	}

	pkt := parsePacket(buf)
	if pkt.isconnack() {
	} else if pkt.isdata() {
		ch := this.chpool.getPool1ById(pkt.Chidsrv)
		if ch == nil {
			ch = this.chpool.getPool2ById(pkt.Conv) // for quick handshake mode
		}
		debug.Println("processing channel data:", ch.chidsrv, len(pkt.Data), gopp.StrSuf(string(pkt.Data), 52))

		buf := pkt.Data
		wn, err := ch.conn.Write(buf)
		if err != nil {
			errl.Println(err)
		}
		debug.Println("kcp->srv:", wn)
		appevt.Trigger("reqbytes", wn, len(buf)+25)
	} else {
	}
}

// should block
func (this *Tunneld) connectToBackend(ch *Channel) {
	//this.chpool.putServer(ch)

	// Dial
	var conn net.Conn
	var err error
	// TODO 如果连接失败，响应的包会导致client崩溃
	if ch.tproto == "tcp" {
		conn, err = net.Dial("tcp", net.JoinHostPort(ch.ip, ch.port))
	} else if ch.tproto == "udp" {
		conn, err = net.Dial("udp", net.JoinHostPort(ch.ip, ch.port))
	} else {
		log.Panicln("not supported proto:", ch.tproto)
	}

	if err != nil {
		errl.Println(err, ch.chidcli, ch.chidsrv, ch.conv, ch.tname)
		// 连接结束
		debug.Println("connection closed, cleaning up...:", ch.chidcli, ch.chidsrv, ch.conv)
		ch.server_socket_close = true
		this.serverCloseChan <- ServerCloseEvent{ch}
		appevt.Trigger("connact", -1)
		return
	}
	ch.conn = conn
	info.Println("connected to:", conn.RemoteAddr().String(), ch.chidcli, ch.chidsrv, ch.conv, ch.tname)
	// info.Println("channel connected,", ch.chidcli, ch.chidsrv, ch.conv, pkt.msgid)

	repkt := ch.makeConnectACKPacket()
	if !this.usemtox {
		_, err = this.FriendSendMessage(ch.toxid, string(repkt.toJson()))
	} else {
		err = this.mtox.sendData(repkt.toJson(), true, true)
	}
	if err != nil {
		debug.Println(err)
	}

	appevt.Trigger("newconn")
	appevt.Trigger("connok")
	appevt.Trigger("connact", 1)
	// can connect backend now，不能阻塞，开新的goroutine
	if rcvs, ok := this.chpool.rcvbuf[ch.conv]; ok && len(rcvs) > 0 {
		delete(this.chpool.rcvbuf, ch.conv)
		log.Println("channel has quick head data:", ch.conv, len(rcvs))
		for i := 0; i < len(rcvs); i++ {
			this.kcpInputChan <- *rcvs[i]
		}
	}
	go this.pollClientReadyRead(ch)
	this.pollServerReadyRead(ch)
}

var spdc2 = mintox.NewSpeedCalc()

func (this *Tunneld) pollServerReadyRead(ch *Channel) {
	lmter := rate.NewLimiter(rate.Limit(1024*1024*2/2), 1024*1024*3/2)
	// TODO 使用内存池
	debug.Println("copying server to client:", ch.chidsrv, ch.chidsrv, ch.conv)
	// 使用kcp的mtu设置了，这里不再需要限制读取的包大小
	for {
		rbuf := make([]byte, rdbufsz)
		n, err := ch.conn.Read(rbuf)
		if err != nil {
			errl.Println(err, ch.chidsrv, ch.chidsrv, ch.conv)
			break
		}

		if false {
			lmter.WaitN(context.Background(), n)
		}

		wn, err := ch.rudp_.Write(rbuf[:n])
		gopp.ErrPrint(err, wn)
		// ch.fp.Write(rbuf[:n])
		if err != nil {
			break
		}

		// 控制kcp.WaitSnd()的大小
		for false {
			if uint32(ch.kcp.WaitSnd()) < ch.kcp.snd_wnd*3 {
				// this.processServerReadyRead(ch, rbuf, n)
				sendbuf := gopp.BytesDup(rbuf[:n])
				this.serverReadyReadChan <- ServerReadyReadEvent{ch, sendbuf, n, false}
				spdc2.Data(n)
				// log.Printf("--- poll srv data speed: %d, WaitSnd:%d, snd_wnd:%d\n",
				// 	spdc2.Avgspd, ch.kcp.WaitSnd(), ch.kcp.snd_wnd)
				break
			} else {
				time.Sleep(3 * time.Millisecond)
			}
		}
	}

	// 连接结束
	debug.Println("connection closed, cleaning up...:", ch.chidcli, ch.chidsrv, ch.conv)
	ch.server_socket_close = true
	this.serverCloseChan <- ServerCloseEvent{ch}
	appevt.Trigger("connact", -1)
}

func (this *Tunneld) pollClientReadyRead(ch *Channel) {
	lmter := rate.NewLimiter(rate.Limit(1024*1024*2/2), 1024*1024*3/2)
	// TODO 使用内存池

	debug.Println("copying client to server:", ch.chidsrv, ch.chidsrv, ch.conv)
	// 使用kcp的mtu设置了，这里不再需要限制读取的包大小
	for {
		rbuf := make([]byte, rdbufsz)
		rn, err := ch.rudp_.Read(rbuf)
		if err != nil {
			errl.Println(err, ch.chidsrv, ch.chidsrv, ch.conv)
			break
		}

		if false {
			lmter.WaitN(context.Background(), rn)
		}

		wn, err := ch.conn.Write(rbuf[:rn])
		gopp.ErrPrint(err, wn)
		if err != nil {
			break
		}
		log.Println("rudp -> tund", rn, wn)

		// 控制kcp.WaitSnd()的大小
		for false {
			if uint32(ch.kcp.WaitSnd()) < ch.kcp.snd_wnd*3 {
				// this.processServerReadyRead(ch, rbuf, n)
				sendbuf := gopp.BytesDup(rbuf[:rn])
				this.serverReadyReadChan <- ServerReadyReadEvent{ch, sendbuf, rn, false}
				spdc2.Data(rn)
				// log.Printf("--- poll srv data speed: %d, WaitSnd:%d, snd_wnd:%d\n",
				// 	spdc2.Avgspd, ch.kcp.WaitSnd(), ch.kcp.snd_wnd)
				break
			} else {
				time.Sleep(3 * time.Millisecond)
			}
		}
	}

	// 连接结束
	debug.Println("connection closed, cleaning up...:", ch.chidcli, ch.chidsrv, ch.conv)
	ch.server_socket_close = true
	this.serverCloseChan <- ServerCloseEvent{ch}
	appevt.Trigger("connact", -1)
}

func (this *Tunneld) processServerReadyRead(ch *Channel, buf []byte, size int) {
	sbuf := buf
	pkt := ch.makeDataPacket(sbuf)
	sn := ch.kcp.Send(pkt.toJson())
	debug.Println("srv->kcp:", sn, size)
	appevt.Trigger("respbytes", size, len(pkt.toJson())+25) // 25 = kcp header len + 1tox
}

func (this *Tunneld) promiseChannelClose(ch *Channel) {
	debug.Println("cleaning up:", ch.chidcli, ch.chidsrv, ch.conv)
	if ch.server_socket_close == true && ch.server_kcp_close == true && ch.client_socket_close == false {
		// server close and no data, connection finished
		pkt := ch.makeCloseFINPacket()
		_, err := this.FriendSendMessage(ch.toxid, string(pkt.toJson()))
		if err != nil {
			// 连接失败
			debug.Println(err)
			return
		}

		ch.addCloseReason("server_close")
		info.Println("server socket closed, kcp empty, close by server",
			ch.chidcli, ch.chidsrv, ch.conv, ch.closeReason())
		this.chpool.rmServer(ch)
		appevt.Trigger("closereason", ch.closeReason())
	} else if ch.server_socket_close == true && ch.server_kcp_close == false && ch.client_socket_close == false {
		ch.addCloseReason("server_close2")
		info.Println("server socket closed, but kcp not empty", ch.chidcli, ch.chidsrv, ch.conv, ch.closeReason())
		appevt.Trigger("closereason", ch.closeReason())
	} else if ch.server_socket_close == false && ch.client_socket_close == true {
		// 客户端先关闭，服务端无条件关闭，比较容易
		ch.addCloseReason("client_close")
		info.Println("force close...", ch.chidcli, ch.chidsrv, ch.conv, ch.closeReason())
		ch.server_socket_close = true // ch.conn真正关闭可能有延时，造成此处重复处理。提前设置关闭标识。
		if ch.conn != nil {
			ch.conn.Close()
			ch.conn = nil
		}
		appevt.Trigger("closereason", ch.closeReason())
	} else if ch.server_socket_close == true && ch.client_socket_close == true {
		ch.addCloseReason("both_close")
		info.Println("both socket closed", ch.chidcli, ch.chidsrv, ch.conv, ch.closeReason())
		this.chpool.rmServer(ch)
		appevt.Trigger("closereason", ch.closeReason())
	} else {
		info.Println("what state:", ch.chidcli, ch.chidsrv, ch.conv,
			ch.server_socket_close, ch.server_kcp_close, ch.client_socket_close)
		panic("Ooops")
	}
}

// TODO
func (this *Tunneld) channelGC() {
	for _, ch := range this.chpool.pool {
		if ch == nil {
		}

	}
}

////////////////
func (this *Tunneld) onToxnetSelfConnectionStatus(t *tox.Tox, status int, extra interface{}) {
	info.Println("mytox status:", status)
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

func (this *Tunneld) onToxnetFriendRequest(t *tox.Tox, friendId string, message string, userData interface{}) {
	debug.Println(friendId, message)

	t.FriendAddNorequest(friendId)
	t.WriteSavedata(tox_savedata_fname)
}

func (this *Tunneld) onToxnetFriendConnectionStatus(t *tox.Tox, friendNumber uint32, status int, userData interface{}) {
	fid, _ := this.tox.FriendGetPublicKey(friendNumber)
	info.Println("peer status (fn/st/id):", friendNumber, status, fid)
	if status == 0 {
		// friendInChannel?
		switchServer(t)
	}
	livebotsOnFriendConnectionStatus(t, friendNumber, status)
	if status == 0 {
		appevt.Trigger("peeronline", false)
		appevt.Trigger("peeroffline")
	} else {
		appevt.Trigger("peeronline", true)
	}
}

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
	type segheader struct {
		conv   uint32
		cmd    uint8
		sn     uint32
		chksum uint32
		datlen uint32
		ts     uint32
	}
	sego := &segheader{}
	seg := sego
	decodeHeader := func(data []byte) error {
		buf := bytes.NewReader(data)
		binary.Read(buf, binary.LittleEndian, &seg.conv)
		binary.Read(buf, binary.LittleEndian, &seg.cmd)
		binary.Read(buf, binary.LittleEndian, &seg.sn)
		binary.Read(buf, binary.LittleEndian, &seg.chksum)
		binary.Read(buf, binary.LittleEndian, &seg.datlen)
		return nil
	}
	// kcp包前4字段为conv，little hacky
	conv := binary.LittleEndian.Uint32(buf)
	ch := this.chpool.getPool2ById(conv)
	if ch == nil {
		// try get sequence no
		// sego := kcp_parse_segment(buf)
		decodeHeader(buf)
		info.Println("channel not found, maybe has some problem, maybe closed", conv, sego.sn, sego.ts, sego.datlen)
		if sego.sn == 0 {
			evt := &ClientReadyReadEvent{nil, buf, len(buf), false}
			this.chpool.rcvbuf[conv] = append(this.chpool.rcvbuf[conv], evt)
		}
	} else {
		// n := ch.kcp.Input(buf, true, true)
		// in := ch.kcp.Input(buf, true, true)
		err := ch.rudp_.Input(buf)
		gopp.ErrPrint(err)
		if err != nil {
			errl.Println("kcp input err:", conv, len(buf))
		} else {
			n := len(buf)
			debug.Println("tox->kcp:", conv, n, len(buf), gopp.StrSuf(string(buf), 52))
		}
	}
}
func (this *Tunneld) handleCtrlPacket(pkt *Packet, friendId string) {
	if pkt.Command == CMDCONNSYN {
		info.Printf("New conn on tunnel %s to %s:%s:%s, conv: %d\n", pkt.Tunname, pkt.Tunproto, pkt.Remoteip, pkt.Remoteport, pkt.Conv)
		ch := NewChannelWithId(pkt.Chidcli, pkt.Tunname)
		ch.tproto = pkt.Tunproto
		ch.conv = this.makeKcpConv(friendId, pkt) // depcreated for quick handshake mode
		ch.conv = pkt.Conv
		ch.ip = pkt.Remoteip
		ch.port = pkt.Remoteport
		ch.toxid = friendId
		// ch.kcp = NewKCP(ch.conv, this.onKcpOutput, ch)
		// ch.kcp.SetMtu(tunmtu)
		// ch.kcp.WndSize(smuse.wndsz, smuse.wndsz)
		// ch.kcp.NoDelay(smuse.nodelay, smuse.interval, smuse.resend, smuse.nc)
		ch.rudp_ = rudp.NewRUDP(ch.conv, func(data []byte, prior bool) error {
			return this.onKcpOutput2(data, len(data), ch, prior)
		})
		// ch.fp, _ = os.OpenFile(fmt.Sprintf("convs%d", ch.conv), os.O_CREATE|os.O_RDWR, 0644)

		this.chpool.putServer(ch)
		go this.connectToBackend(ch)

	} else if pkt.Command == CMDCLOSEFIN {
		ch := this.chpool.getPool2ById(pkt.Conv)
		if ch != nil {
			info.Println("recv client close fin,", ch.chidcli, ch.chidsrv, ch.conv, pkt.Msgid)
			ch.client_socket_close = true
			this.promiseChannelClose(ch)
		} else {
			info.Println("recv client close fin, but maybe server already closed",
				pkt.Command, pkt.Chidcli, pkt.Chidsrv, pkt.Conv, pkt.Msgid)
		}
	} else {
		errl.Println("wtf, unknown cmmand:", pkt.Command, pkt.Chidcli, pkt.Chidsrv, pkt.Conv)
	}
}

// a tool function
func (this *Tunneld) makeKcpConv(friendId string, pkt *Packet) uint32 {
	// crc32: toxid+host+port+time
	return makeKcpConv(friendId, pkt.Remoteip, pkt.Remoteport)
}
func (this *Tunneld) onToxnetFriendMessage(t *tox.Tox, friendNumber uint32, message string, userData interface{}) {
	debug.Println(friendNumber, len(message), gopp.StrSuf(message, 52))
	friendId, err := this.tox.FriendGetPublicKey(friendNumber)
	if err != nil {
		errl.Println(err)
	}

	pkt := parsePacket(bytes.NewBufferString(message).Bytes())
	if pkt == nil {
		info.Println("maybe not command, just normal message", gopp.StrSuf(message, 52))
	} else {
		this.handleCtrlPacket(pkt, friendId)
	}
}

func (this *Tunneld) onToxnetFriendLossyPacket(t *tox.Tox, friendNumber uint32, message string, userData interface{}) {
	debug.Println(friendNumber, len(message), gopp.StrSuf(message, 52), time.Now().String())
	buf := bytes.NewBufferString(message).Bytes()
	if buf[0] == 254 {
		this.handleDataPacket(buf[1:], friendNumber)
	} else {
		info.Println("unknown message:", buf[0])
	}
}

func (this *Tunneld) onToxnetFriendLosslessPacket(t *tox.Tox, friendNumber uint32, message string, userData interface{}) {
	debug.Println(friendNumber, len(message), gopp.StrSuf(message, 52))
	buf := bytes.NewBufferString(message).Bytes()
	if buf[0] == 191 {
		buf = buf[1:]
		// kcp包前4字节为conv，little hacky
		if len(buf) < 4 {
			errl.Println("wtf")
		}
		conv := binary.LittleEndian.Uint32(buf)
		ch := this.chpool.getPool2ById(conv)
		if ch == nil {
			errl.Println("channel not found, maybe has some problem, maybe already closed", conv)
		} else {
			n := ch.kcp.Input(buf, true, true)
			debug.Println("tox->kcp:", conv, n, len(buf), gopp.StrSuf(string(buf), 52))
		}
	} else {
		info.Println("unknown message:", buf[0])
	}
}

func (this *Tunneld) FriendSendMessage(friendId string, message string) (uint32, error) {
	friendNumber, err := this.tox.FriendByPublicKey(friendId)
	if err != nil {
		return 0, err
	}
	return this.tox.FriendSendMessage(friendNumber, message)
}

func (this *Tunneld) FriendSendLossyPacket(friendId string, message string) error {
	friendNumber, err := this.tox.FriendByPublicKey(friendId)
	if err != nil {
		return err
	}
	return this.tox.FriendSendLossyPacket(friendNumber, message)
}

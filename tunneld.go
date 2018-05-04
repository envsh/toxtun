package main

import (
	"fmt"
	"math"
	// "log"
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"hash/crc32"
	"net"
	"time"

	tox "github.com/TokTok/go-toxcore-c"
	"github.com/kitech/goplusplus"
)

type Tunneld struct {
	tox    *tox.Tox
	chpool *ChannelPool

	kcpNextUpdateWait int

	toxPollChan chan ToxPollEvent
	// toxReadyReadChan    chan ToxReadyReadEvent
	// toxMessageChan      chan ToxMessageEvent
	kcpPollChan       chan KcpPollEvent
	kcpCheckCloseChan chan KcpCheckCloseEvent
	// kcpReadyReadChan    chan KcpReadyReadEvent
	// kcpOutputChan       chan KcpOutputEvent
	serverReadyReadChan chan ServerReadyReadEvent
	serverCloseChan     chan ServerCloseEvent
	channelGCChan       chan ChannelGCEvent
}

func NewTunneld() *Tunneld {
	this := new(Tunneld)
	this.chpool = NewChannelPool()

	t := makeTox("toxtund")
	this.tox = t

	// callbacks
	t.CallbackSelfConnectionStatus(this.onToxnetSelfConnectionStatus, nil)
	t.CallbackFriendRequest(this.onToxnetFriendRequest, nil)
	t.CallbackFriendConnectionStatus(this.onToxnetFriendConnectionStatus, nil)
	t.CallbackFriendMessage(this.onToxnetFriendMessage, nil)
	t.CallbackFriendLossyPacket(this.onToxnetFriendLossyPacket, nil)
	t.CallbackFriendLosslessPacket(this.onToxnetFriendLosslessPacket, nil)

	///
	return this
}

func (this *Tunneld) serve() {

	this.toxPollChan = make(chan ToxPollEvent, mpcsz)
	// this.toxReadyReadChan = make(chan ToxReadyReadEvent, 0)
	// this.toxMessageChan = make(chan ToxMessageEvent, 0)
	this.kcpPollChan = make(chan KcpPollEvent, mpcsz)
	this.kcpCheckCloseChan = make(chan KcpCheckCloseEvent, mpcsz)
	// this.kcpReadyReadChan = make(chan KcpReadyReadEvent, 0)
	// this.kcpOutputChan = make(chan KcpOutputEvent, 0)
	this.channelGCChan = make(chan ChannelGCEvent, mpcsz)
	this.serverReadyReadChan = make(chan ServerReadyReadEvent, mpcsz)
	this.serverCloseChan = make(chan ServerCloseEvent, mpcsz)

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
			if this.kcpNextUpdateWait > 0 {
				time.Sleep(time.Duration(this.kcpNextUpdateWait) * time.Millisecond)
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
		case evt := <-this.serverReadyReadChan:
			this.processServerReadyRead(evt.ch, evt.buf, evt.size)
		case evt := <-this.serverCloseChan:
			this.promiseChannelClose(evt.ch)
		case <-this.toxPollChan:
			iterate(this.tox)
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
func kcp_poll(pool map[int]*Channel) (chks []*Channel, nxtss []uint32) {
	for _, ch := range pool {
		if ch.kcp == nil {
			continue
		}
		curts := uint32(iclock())
		rts := ch.kcp.Check(curts)
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
			this.kcpNextUpdateWait = int(mints)
			return
		} else {
			this.kcpNextUpdateWait = 10
		}

		for _, ch := range chks {
			if ch.kcp == nil {
				continue
			}

			ch.kcp.Update(uint32(iclock()))

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

func (this *Tunneld) onKcpOutput(buf []byte, size int, extra interface{}) {
	if size <= 0 {
		// 如果总是出现，并且不影响程序运行，那么也就不是bug了
		// info.Println("wtf")
		return
	}
	debug.Println(len(buf), "//", size, "//", string(gopp.SubBytes(buf, 52)))
	ch := extra.(*Channel)

	msg := string([]byte{254}) + string(buf[:size])
	err := this.FriendSendLossyPacket(ch.toxid, msg)
	// msg := string([]byte{191}) + string(buf[:size])
	// err := this.tox.FriendSendLosslessPacket(0, msg)
	if err != nil {
		debug.Println(err)
	} else {
		debug.Println("kcp->tox:", len(msg), time.Now().String())
	}
}

func (this *Tunneld) processKcpReadyRead(ch *Channel) {
	if ch.conn == nil {
		errl.Println("Not Connected:", ch.chidsrv, ch.chidcli)
		// return
	}

	buf := make([]byte, ch.kcp.PeekSize())
	n := ch.kcp.Recv(buf)

	if len(buf) != n {
		errl.Println("Invalide kcp recv data")
	}

	pkt := parsePacket(buf)
	if pkt.isconnack() {
	} else if pkt.isdata() {
		ch := this.chpool.pool[pkt.chidsrv]
		debug.Println("processing channel data:", ch.chidsrv, len(pkt.data), gopp.StrSuf(pkt.data, 52))
		buf, err := base64.StdEncoding.DecodeString(pkt.data)
		if err != nil {
			errl.Println(err)
		}

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
	this.chpool.putServer(ch)

	// Dial
	conn, err := net.Dial("tcp", net.JoinHostPort(ch.ip, ch.port))
	if err != nil {
		errl.Println(err, ch.chidcli, ch.chidsrv, ch.conv)
		// 连接结束
		debug.Println("connection closed, cleaning up...:", ch.chidcli, ch.chidsrv, ch.conv)
		ch.server_socket_close = true
		this.serverCloseChan <- ServerCloseEvent{ch}
		appevt.Trigger("connact", -1)
		return
	}
	ch.conn = conn
	info.Println("connected to:", conn.RemoteAddr().String(), ch.chidcli, ch.chidsrv, ch.conv)
	// info.Println("channel connected,", ch.chidcli, ch.chidsrv, ch.conv, pkt.msgid)

	repkt := ch.makeConnectACKPacket()
	r, err := this.FriendSendMessage(ch.toxid, string(repkt.toJson()))
	if err != nil {
		debug.Println(err, r)
	}

	appevt.Trigger("newconn")
	appevt.Trigger("connok")
	appevt.Trigger("connact", 1)
	// can connect backend now，不能阻塞，开新的goroutine
	this.pollServerReadyRead(ch)
}

func (this *Tunneld) pollServerReadyRead(ch *Channel) {
	// TODO 使用内存池
	rbuf := make([]byte, rdbufsz)

	debug.Println("copying server to client:", ch.chidsrv, ch.chidsrv, ch.conv)
	// 使用kcp的mtu设置了，这里不再需要限制读取的包大小
	for {
		n, err := ch.conn.Read(rbuf)
		if err != nil {
			errl.Println(err, ch.chidsrv, ch.chidsrv, ch.conv)
			break
		}

		// 控制kcp.WaitSnd()的大小
		for {
			if uint32(ch.kcp.WaitSnd()) < ch.kcp.snd_wnd*5 {
				// this.processServerReadyRead(ch, rbuf, n)
				sendbuf := gopp.BytesDup(rbuf[:n])
				this.serverReadyReadChan <- ServerReadyReadEvent{ch, sendbuf, n}
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
	sbuf := base64.StdEncoding.EncodeToString(buf[:size])
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
		ch.conn.Close()
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

	if status == 0 {
		appevt.Trigger("peeronline", false)
		appevt.Trigger("peeroffline")
	} else {
		appevt.Trigger("peeronline", true)
	}
}

// a tool function
func (this *Tunneld) makeKcpConv(friendId string, pkt *Packet) uint32 {
	// crc32: toxid+host+port+time
	data := fmt.Sprintf("%s@%s:%s@%d", friendId, pkt.remoteip, pkt.remoteport,
		time.Now().UnixNano())
	conv := crc32.ChecksumIEEE(bytes.NewBufferString(data).Bytes())
	return conv
}
func (this *Tunneld) onToxnetFriendMessage(t *tox.Tox, friendNumber uint32, message string, userData interface{}) {
	debug.Println(friendNumber, len(message), gopp.StrSuf(message, 52))
	friendId, err := this.tox.FriendGetPublicKey(friendNumber)
	if err != nil {
		errl.Println(err)
	}

	pkt := parsePacket(bytes.NewBufferString(message).Bytes())
	if pkt == nil {
		info.Println("maybe not command, just normal message")
	} else {
		if pkt.command == CMDCONNSYN {
			ch := NewChannelWithId(pkt.chidcli)
			ch.conv = this.makeKcpConv(friendId, pkt)
			ch.ip = pkt.remoteip
			ch.port = pkt.remoteport
			ch.toxid = friendId
			ch.kcp = NewKCP(ch.conv, this.onKcpOutput, ch)
			ch.kcp.SetMtu(tunmtu)
			if kcp_mode == "fast" {
				ch.kcp.WndSize(128, 128)
				ch.kcp.NoDelay(1, 10, 2, 1)
			}

			go this.connectToBackend(ch)

		} else if pkt.command == CMDCLOSEFIN {
			if ch, ok := this.chpool.pool2[pkt.conv]; ok {
				info.Println("recv client close fin,", ch.chidcli, ch.chidsrv, ch.conv, pkt.msgid)
				ch.client_socket_close = true
				this.promiseChannelClose(ch)
			} else {
				info.Println("recv client close fin, but maybe server already closed",
					pkt.command, pkt.chidcli, pkt.chidsrv, pkt.conv, pkt.msgid)
			}
		} else {
			errl.Println("wtf, unknown cmmand:", pkt.command, pkt.chidcli, pkt.chidsrv, pkt.conv)
		}

	}
}

func (this *Tunneld) onToxnetFriendLossyPacket(t *tox.Tox, friendNumber uint32, message string, userData interface{}) {
	debug.Println(friendNumber, len(message), gopp.StrSuf(message, 52), time.Now().String())
	buf := bytes.NewBufferString(message).Bytes()
	if buf[0] == 254 {
		buf = buf[1:]
		// kcp包前4字段为conv，little hacky
		conv := binary.LittleEndian.Uint32(buf)
		ch := this.chpool.pool2[conv]
		if ch == nil {
			info.Println("channel not found, maybe has some problem, maybe closed", conv)
		} else {
			n := ch.kcp.Input(buf)
			debug.Println("tox->kcp:", conv, n, len(buf), gopp.StrSuf(string(buf), 52))
		}
	} else {
		info.Println("unknown message:", buf[0])
	}
}

func (this *Tunneld) onToxnetFriendLosslessPacket(t *tox.Tox, friendNumber uint32, message string, userData interface{}) {
	debug.Println(friendNumber, len(message), gopp.StrSuf(message, 52))
	buf := bytes.NewBufferString(message).Bytes()
	if buf[0] == 191 {
		buf = buf[1:]
		// kcp包前4字段为conv，little hacky
		if len(buf) < 4 {
			errl.Println("wtf")
		}
		conv := binary.LittleEndian.Uint32(buf)
		ch := this.chpool.pool2[conv]
		if ch == nil {
			errl.Println("channel not found, maybe has some problem, maybe already closed", conv)
		} else {
			n := ch.kcp.Input(buf)
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

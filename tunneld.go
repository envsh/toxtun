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

	"gopp"
	"tox"
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

	mpcsz := 256
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
			time.Sleep(30 * time.Millisecond)
			this.toxPollChan <- ToxPollEvent{}
			//iterate(this.tox)
		}
	}()
	go func() {
		for {
			if this.kcpNextUpdateWait > 0 {
				time.Sleep(time.Duration(this.kcpNextUpdateWait) * time.Millisecond)
			} else {
				time.Sleep(30 * time.Millisecond)
			}
			this.kcpPollChan <- KcpPollEvent{}
			//this.serveKcp()
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
func (this *Tunneld) serveKcp() {
	zbuf := make([]byte, 0)
	nxts := make([]uint32, 0)
	chks := make(map[*Channel]bool, 0)
	if true {
		for _, ch := range this.chpool.pool {
			if ch.kcp == nil {
				continue
			}
			curts := uint32(iclock())
			rts := ch.kcp.Check(curts)
			if rts == curts {
				nxts = append(nxts, 10)
				chks[ch] = true
			} else {
				nxts = append(nxts, rts-curts)
			}
		}

		mints := gopp.MinU32(nxts)
		if mints > 10 && mints != math.MaxUint32 {
			this.kcpNextUpdateWait = int(mints)
			return
		} else {
			this.kcpNextUpdateWait = 10
		}

		for _, ch := range this.chpool.pool {
			if ch.kcp == nil {
				continue
			}
			if _, ok := chks[ch]; !ok {
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
		errl.Println(err)
	}
	info.Println("kcp->tox:", len(msg), time.Now().String())
}

func (this *Tunneld) processKcpReadyRead(ch *Channel) {
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
		info.Println("kcp->srv:", wn)
	} else {
	}
}

func (this *Tunneld) pollServerReadyRead(ch *Channel) {
	// Dial
	conn, err := net.Dial("tcp", net.JoinHostPort(ch.ip, ch.port))
	if err != nil {
		errl.Println(err)
	}
	ch.conn = conn
	info.Println("connected to:", conn.RemoteAddr().String())

	debug.Println("copying server to client:", ch.chidsrv)
	// 使用kcp的mtu设置了，这里不再需要限制读取的包大小
	// TODO 使用内存池
	rbuf := make([]byte, rdbufsz)
	for {
		n, err := ch.conn.Read(rbuf)
		if err != nil {
			errl.Println(err)
			break
		}

		// this.processServerReadyRead(ch, rbuf, n)
		btime := time.Now()
		debug.Println(btime.String())
		sendbuf := gopp.BytesDup(rbuf[:n])
		this.serverReadyReadChan <- ServerReadyReadEvent{ch, sendbuf, n}
		etime := time.Now()
		debug.Println(etime.String(), etime.Sub(btime).String())
	}

	// 连接结束
	info.Println("connection closed, cleaning up...:", ch.chidcli, ch.chidsrv, ch.conv)
	ch.server_socket_close = true
	this.serverCloseChan <- ServerCloseEvent{ch}
}

func (this *Tunneld) processServerReadyRead(ch *Channel, buf []byte, size int) {
	sbuf := base64.StdEncoding.EncodeToString(buf[:size])
	pkt := ch.makeDataPacket(sbuf)
	sn := ch.kcp.Send(pkt.toJson())
	info.Println("srv->kcp:", sn, size)
}

func (this *Tunneld) promiseChannelClose(ch *Channel) {
	debug.Println("cleaning up:", ch.chidcli, ch.chidsrv, ch.conv)
	if ch.server_socket_close == true && ch.server_kcp_close == true && ch.client_socket_close == false {
		// server close and no data, connection finished
		info.Println("server socket closed, kcp empty, close by server",
			ch.chidcli, ch.chidsrv, ch.conv)
		pkt := ch.makeCloseACKPacket()
		n, err := this.FriendSendMessage(ch.toxid, string(pkt.toJson()))
		if err != nil {
			// 连接失败
			errl.Println(err)
			return
		}

		info.Println(n, gopp.SubStr(string(pkt.toJson()), 152))
		this.chpool.rmServer(ch)
	} else if ch.server_socket_close == true && ch.server_kcp_close == false && ch.client_socket_close == false {
		info.Println("server socket closed, but kcp not empty", ch.chidcli, ch.chidsrv, ch.conv)
	} else if ch.server_socket_close == false && ch.client_socket_close == true {
		// 客户端先关闭，服务端无条件关闭，比较容易
		info.Println("force close...", ch.chidcli, ch.chidsrv, ch.conv)
		ch.conn.Close()
	} else if ch.server_socket_close == true && ch.client_socket_close == true {
		info.Println("both socket closed", ch.chidcli, ch.chidsrv, ch.conv)
		this.chpool.rmServer(ch)
		// ch.kcp.Close
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
func (this *Tunneld) onToxnetSelfConnectionStatus(t *tox.Tox, status uint32, extra interface{}) {
	info.Println(status)
}

func (this *Tunneld) onToxnetFriendRequest(t *tox.Tox, friendId string, message string, userData interface{}) {
	debug.Println(friendId, message)

	t.FriendAddNorequest(friendId)
	t.WriteSavedata(fname)
}

func (this *Tunneld) onToxnetFriendConnectionStatus(t *tox.Tox, friendNumber uint32, status uint32, userData interface{}) {
	fid, _ := this.tox.FriendGetPublicKey(friendNumber)
	info.Println(friendNumber, status, fid)
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
		if pkt.command == CMDCONNACK {
			ch := NewChannelWithId(pkt.chidcli)
			ch.conv = this.makeKcpConv(friendId, pkt)
			ch.ip = pkt.remoteip
			ch.port = pkt.remoteport
			ch.toxid = friendId
			ch.kcp = NewKCP(ch.conv, this.onKcpOutput, ch)
			ch.kcp.SetMtu(tunmtu)
			// ch.kcp.WndSize(128, 128)
			// ch.kcp.NoDelay(1, 10, 2, 1)
			this.chpool.putServer(ch)

			info.Println("channel connected,", ch.chidcli, ch.chidsrv, ch.conv)

			repkt := ch.makeConnectFINPacket()
			r, err := this.FriendSendMessage(ch.toxid, string(repkt.toJson()))
			if err != nil {
				errl.Println(err, r)
			}

			// can connect backend now，不能阻塞，开新的goroutine
			go this.pollServerReadyRead(ch)
		} else if pkt.command == CMDCLOSEACK {
			if ch, ok := this.chpool.pool2[pkt.conv]; ok {
				ch.client_socket_close = true
				this.promiseChannelClose(ch)
			} else {
				info.Println("recv client close ack, but maybe server already closed",
					pkt.command, pkt.chidcli, pkt.chidsrv, pkt.conv)
			}
		} else if pkt.command == CMDCLOSEFIN1 {
			ch := this.chpool.pool2[pkt.conv]
			ch.client_socket_close = true
			this.promiseChannelClose(ch)
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
		var conv uint32
		// kcp包前4字段为conv，little hacky
		conv = binary.LittleEndian.Uint32(buf)
		ch := this.chpool.pool2[conv]
		debug.Println("conv:", conv, ch)
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
		var conv uint32
		// kcp包前4字段为conv，little hacky
		conv = binary.LittleEndian.Uint32(buf)
		ch := this.chpool.pool2[conv]
		debug.Println("conv:", conv, ch)
		if ch == nil {
			info.Println("channel not found, maybe has some problem, maybe already closed", conv)
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

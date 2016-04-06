package main

import (
	"fmt"
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

	toxPollChan         chan ToxPollEvent
	toxReadyReadChan    chan ToxReadyReadEvent
	toxMessageChan      chan ToxMessageEvent
	kcpPollChan         chan KcpPollEvent
	kcpReadyReadChan    chan KcpReadyReadEvent
	kcpOutputChan       chan KcpOutputEvent
	serverReadyReadChan chan ServerReadyReadEvent
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

	///
	return this
}

func (this *Tunneld) serve() {

	this.toxPollChan = make(chan ToxPollEvent, 0)
	this.toxReadyReadChan = make(chan ToxReadyReadEvent, 0)
	this.toxMessageChan = make(chan ToxMessageEvent, 0)
	this.kcpPollChan = make(chan KcpPollEvent, 0)
	this.kcpReadyReadChan = make(chan KcpReadyReadEvent, 0)
	this.kcpOutputChan = make(chan KcpOutputEvent, 0)
	this.serverReadyReadChan = make(chan ServerReadyReadEvent, 0)

	// install pollers
	go func() {
		for {
			time.Sleep(1000 * 50 * time.Microsecond)
			this.toxPollChan <- ToxPollEvent{}
		}
	}()
	go func() {
		for {
			time.Sleep(1000 * 30 * time.Microsecond)
			this.kcpPollChan <- KcpPollEvent{}
		}
	}()

	// like event handler
	for {
		select {
		case <-this.toxPollChan:
			iterate(this.tox)
		case evt := <-this.toxReadyReadChan:
			this.processFriendLossyPacket(this.tox, evt.friendNumber, evt.message, nil)
		case evt := <-this.toxMessageChan:
			this.processFriendMessage(this.tox, evt.friendNumber, evt.message, nil)
		case <-this.kcpPollChan:
			this.serveKcp(this.kcpReadyReadChan)
		case evt := <-this.kcpReadyReadChan:
			this.processKcpReadyRead(evt.ch)
		case evt := <-this.kcpOutputChan:
			this.processKcpOutput(evt.buf, evt.size, evt.extra)
		case evt := <-this.serverReadyReadChan:
			this.processServerReadyRead(evt.ch, evt.buf, evt.size)
		}
	}

	// go this.serveKcp()
	// iterate(this.tox)
}

///////////
func (this *Tunneld) serveKcp(kcpReadyReadChan chan KcpReadyReadEvent) {
	zbuf := make([]byte, 0)
	if true {
		for _, ch := range this.chpool.pool {
			if ch.kcp == nil {
				continue
			}

			ch.kcp.Update(uint32(iclock()))

			n := ch.kcp.Recv(zbuf)
			switch n {
			case -3:
				kcpReadyReadChan <- KcpReadyReadEvent{ch}
			case -2: // just empty kcp recv queue
				// errl.Println("kcp recv internal error:", n, this.kcp.PeekSize())
			case -1: // EAGAIN
			default:
				errl.Println("unknown recv:", n)
			}
		}
	}
}

func (this *Tunneld) onKcpOutput(buf []byte, size int, extra interface{}) {
	debug.Println(len(buf), size, string(gopp.SubBytes(buf, 52)))
	this.kcpOutputChan <- KcpOutputEvent{buf, size, extra}
}
func (this *Tunneld) processKcpOutput(buf []byte, size int, extra interface{}) {
	debug.Println(len(buf), size, string(gopp.SubBytes(buf, 52)))

	if size <= 0 {
		info.Println("wtf")
		return
	}

	msg := string([]byte{254}) + string(buf[:size])
	err := this.tox.FriendSendLossyPacket(0, msg)
	if err != nil {
		errl.Println(err)
	}
	info.Println("kcp->tox:", len(msg))
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
	rbuf := make([]byte, rdbufsz)
	for {
		n, err := ch.conn.Read(rbuf)
		if err != nil {
			errl.Println(err)
			break
		}

		this.serverReadyReadChan <- ServerReadyReadEvent{ch, rbuf, n}
	}
}
func (this *Tunneld) processServerReadyRead(ch *Channel, buf []byte, size int) {
	sbuf := base64.StdEncoding.EncodeToString(buf[:size])
	pkt := ch.makeDataPacket(sbuf)
	sn := ch.kcp.Send(pkt.toJson())
	info.Println("srv->kcp:", sn)
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
	// toxid+host+port+time
	data := fmt.Sprintf("%s@%s:%s@%d", friendId, pkt.remoteip, pkt.remoteport,
		time.Now().UnixNano())
	conv := crc32.ChecksumIEEE(bytes.NewBufferString(data).Bytes())
	return conv
}
func (this *Tunneld) onToxnetFriendMessage(t *tox.Tox, friendNumber uint32, message string, userData interface{}) {
	debug.Println(friendNumber, len(message), gopp.StrSuf(message, 52))
	this.toxMessageChan <- ToxMessageEvent{friendNumber, message}
}
func (this *Tunneld) processFriendMessage(t *tox.Tox, friendNumber uint32, message string, userData interface{}) {
	debug.Println(friendNumber, len(message), gopp.StrSuf(message, 52))

	friendId, err := this.tox.FriendGetPublicKey(friendNumber)
	if err != nil {
		errl.Println(err)
	}

	pkt := parsePacket(bytes.NewBufferString(message).Bytes())
	if pkt == nil {
		info.Println("maybe not command, just normal message")
	} else {
		ch := NewChannelWithId(pkt.chidcli)
		ch.conv = this.makeKcpConv(friendId, pkt)
		ch.ip = pkt.remoteip
		ch.port = pkt.remoteport
		ch.toxid = friendId
		ch.kcp = NewKCP(ch.conv, this.onKcpOutput, ch)
		ch.kcp.SetMtu(tunmtu)
		this.chpool.putServer(ch)

		info.Println("channel connected,", ch.chidcli, ch.chidsrv, ch.conv)

		repkt := ch.makeConnectFINPacket()
		r, err := this.tox.FriendSendMessage(friendNumber, string(repkt.toJson()))
		if err != nil {
			errl.Println(err, r)
		}

		// can connect backend now，不能阻塞，开新的goroutine
		go this.pollServerReadyRead(ch)
	}
}

func (this *Tunneld) onToxnetFriendLossyPacket(t *tox.Tox, friendNumber uint32, message string, userData interface{}) {
	debug.Println(friendNumber, len(message), gopp.StrSuf(message, 52))
	this.toxReadyReadChan <- ToxReadyReadEvent{friendNumber, message}
}
func (this *Tunneld) processFriendLossyPacket(t *tox.Tox, friendNumber uint32, message string, userData interface{}) {
	debug.Println(friendNumber, len(message), gopp.StrSuf(message, 52))

	buf := bytes.NewBufferString(message).Bytes()
	if buf[0] == 254 {
		buf = buf[1:]
		var conv uint32
		// kcp包前4字段为conv，little hacky
		conv = binary.LittleEndian.Uint32(buf)
		ch := this.chpool.pool2[conv]
		debug.Println("conv:", conv, ch)
		if ch == nil {
			info.Println("maybe has some problem")
		}
		n := ch.kcp.Input(buf)
		debug.Println("tox->kcp:", conv, n, len(buf), gopp.StrSuf(string(buf), 52))
	} else {
		info.Println("unknown message:", buf[0])
	}
}

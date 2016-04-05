package main

import (
	// "fmt"
	// "log"
	"encoding/base64"
	"net"
	"time"
	"unsafe"

	"gopp"
	"tox"
)

type Tunneld struct {
	tox    *tox.Tox
	kcp    *KCP
	chpool *ChannelPool
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

	///
	this.kcp = NewKCP(tunconv, this.onKcpOutput, this)
	this.kcp.SetMtu(tunmtu)
	return this
}

func (this *Tunneld) serve() {
	go this.serveKcp()
	iterate(this.tox)
}

///////////
func (this *Tunneld) serveKcp() {
	zbuf := make([]byte, 0)
	for {
		time.Sleep(1000 * 30 * time.Microsecond)
		this.kcp.Update(uint32(iclock()))

		n := this.kcp.Recv(zbuf)
		switch n {
		case -3:
			rbuf := make([]byte, this.kcp.PeekSize())
			n := this.kcp.Recv(rbuf)
			go this.processKcpReceive(rbuf, n)
		case -2: // just empty kcp recv queue
			// errl.Println("kcp recv internal error:", n, this.kcp.PeekSize())
		case -1: // EAGAIN
		default:
			errl.Println("unknown recv:", n)
		}
	}
}

func (this *Tunneld) onKcpOutput(buf []byte, size int, extra interface{}) {
	debug.Println(len(buf), size, string(gopp.SubBytes(buf, 32)))

	msg := base64.StdEncoding.EncodeToString(buf[:size])
	n, err := this.tox.FriendSendMessage(0, msg)
	if err != nil {
		errl.Println(err, n)
		debug.Println(string(buf[:size]))
	}
	info.Println("kcp->tox:", len(msg))
}

func (this *Tunneld) processKcpReceive(buf []byte, n int) {
	if len(buf) != n {
		errl.Println("Invalide kcp recv data")
	}

	pkt := parsePacket(buf)
	if pkt.isconnack() {
		ch := NewChannelWithId(pkt.chidcli)
		ch.ip = pkt.remoteip
		ch.port = pkt.remoteport
		this.chpool.putServer(ch)

		repkt := ch.makeConnectFINPacket()
		this.kcp.Send(repkt.toJson())

		// Dial
		conn, err := net.Dial("tcp", net.JoinHostPort(ch.ip, ch.port))
		if err != nil {
			errl.Println(err)
		}
		ch.conn = conn
		info.Println("connected to:", conn.RemoteAddr().String())
		go this.copyServer2Client(ch)
	} else if pkt.isdata() {
		ch := this.chpool.pool[pkt.chidsrv]
		this.processChannel(ch, pkt)
	} else {
	}
}

func (this *Tunneld) processChannel(ch *Channel, pkt *Packet) {
	debug.Println("processing channel data:", ch.chidsrv, len(pkt.data), gopp.StrSuf(pkt.data, 32))
	buf, err := base64.StdEncoding.DecodeString(pkt.data)
	if err != nil {
		errl.Println(err)
	}

	wn, err := ch.conn.Write(buf)
	if err != nil {
		errl.Println(err)
	}
	info.Println("kcp->srv:", wn)
}

func (this *Tunneld) copyServer2Client(ch *Channel) {
	debug.Println("copying server to client:", ch.chidsrv)
	// 使用kcp的mtu设置了，这里不再需要限制读取的包大小
	rbuf := make([]byte, rdbufsz)
	for {
		n, err := ch.conn.Read(rbuf)
		if err != nil {
			errl.Println(err)
			break
		}

		sbuf := base64.StdEncoding.EncodeToString(rbuf[:n])
		pkt := ch.makeDataPacket(sbuf)
		sn := this.kcp.Send(pkt.toJson())
		info.Println("srv->kcp:", sn)
	}
}

////////////////
func (this *Tunneld) onToxnetSelfConnectionStatus(t *tox.Tox, status uint32, extra unsafe.Pointer) {
	info.Println(status)
}

func (this *Tunneld) onToxnetFriendRequest(t *tox.Tox, friendId string, message string, userData unsafe.Pointer) {
	debug.Println(friendId, message)

	t.FriendAddNorequest(friendId)
	t.WriteSavedata(fname)
}

func (this *Tunneld) onToxnetFriendConnectionStatus(t *tox.Tox, friendNumber uint32, status uint32, userData unsafe.Pointer) {
	fid, _ := this.tox.FriendGetPublicKey(friendNumber)
	info.Println(friendNumber, status, fid)
}

func (this *Tunneld) onToxnetFriendMessage(t *tox.Tox, friendNumber uint32, message string, userData unsafe.Pointer) {
	debug.Println(friendNumber, len(message), gopp.StrSuf(message, 32))
	buf, err := base64.StdEncoding.DecodeString(message)
	if err != nil {
		errl.Println(err)
	}

	n := this.kcp.Input(buf)
	debug.Println("tox->kcp:", n, len(buf))
}

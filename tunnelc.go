package main

import (
	"fmt"
	"net"
	// "strings"
	"time"
	// "bytes"
	"encoding/base64"
	"unsafe"

	"gopp"
	"tox"
)

const (
	server_port = 8113
)

type Tunnelc struct {
	tox    *tox.Tox
	kcp    *KCP
	srv    net.Listener
	chpool *ChannelPool
}

func NewTunnelc() *Tunnelc {
	this := new(Tunnelc)
	this.chpool = NewChannelPool()

	t := makeTox("toxtunc")
	this.tox = t

	// callbacks
	t.CallbackSelfConnectionStatus(this.onToxnetSelfConnectionStatus, nil)
	t.CallbackFriendRequest(this.onToxnetFriendRequest, nil)
	t.CallbackFriendConnectionStatus(this.onToxnetFriendConnectionStatus, nil)
	t.CallbackFriendMessage(this.onToxnetFriendMessage, nil)

	//
	this.kcp = NewKCP(tunconv, this.onKcpOutput, this)
	this.kcp.SetMtu(tunmtu)
	return this
}

func (this *Tunnelc) serve() {
	srv, err := net.Listen("tcp", fmt.Sprintf(":%d", server_port))
	if err != nil {
		info.Println(err)
		return
	}
	this.srv = srv
	info.Println("tunaddr:", srv.Addr().String())

	go iterate(this.tox)
	go this.serveKcp()
	this.serveTcp()
}

func (this *Tunnelc) serveTcp() {
	srv := this.srv

	for {
		c, err := srv.Accept()
		if err != nil {
			info.Println(err)
		}
		// info.Println(c)
		this.serveConn(c)
	}
}

func (this *Tunnelc) serveConn(conn net.Conn) {
	ch := NewChannelClient(conn)
	ch.ip = "127.0.0.1"
	ch.port = "8118"
	this.chpool.putClient(ch)

	pkt := ch.makeConnectACKPacket()

	n := this.kcp.Send(pkt.toJson())
	debug.Println(n, gopp.SubStr(string(pkt.toJson()), 32))
	// c.Close()
}

//////////
func iclock() int32 {
	return int32((time.Now().UnixNano() / 1000000) & 0xffffffff)
}
func (this *Tunnelc) serveKcp() {
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

func (this *Tunnelc) onKcpOutput(buf []byte, size int, extra interface{}) {
	debug.Println(len(buf), size, string(gopp.SubBytes(buf, 32)))

	msg := base64.StdEncoding.EncodeToString(buf[0:size])
	// msg = string(byte(254)) + msg
	debug.Println(msg)
	this.tox.FriendSendMessage(0, msg)

	/*
		err := this.tox.FriendSendLossyPacket(0, msg)
		if err != nil {
			errl.Println(err)
		}
	*/
}

func (this *Tunnelc) processKcpReceive(buf []byte, n int) {
	if len(buf) != n {
		errl.Println("Invalide kcp recv data")
	}

	pkt := parsePacket(buf)
	if pkt.isconnfin() {
		ch := this.chpool.pool[pkt.chidcli]
		ch.chidsrv = pkt.chidsrv
		info.Println("channel connected:", ch.chidcli)
		go this.processChannel(ch)
	} else if pkt.isdata() {
		ch := this.chpool.pool[pkt.chidcli]
		this.copyServer2Client(ch, pkt)
	} else {
	}

}

func (this *Tunnelc) processChannel(ch *Channel) {
	this.copyClient2Server(ch)
}

func (this *Tunnelc) copyClient2Server(ch *Channel) {
	// 使用kcp的mtu设置了，这里不再需要限制读取的包大小
	rbuf := make([]byte, rdbufsz)
	for {
		n, err := ch.conn.Read(rbuf)
		if err != nil {
			info.Println("chan read:", err)
			break
		}

		sbuf := base64.StdEncoding.EncodeToString(rbuf[:n])
		pkt := ch.makeDataPacket(sbuf)
		sn := this.kcp.Send(pkt.toJson())
		info.Println("cli->kcp:", sn)
	}
}

func (this *Tunnelc) copyServer2Client(ch *Channel, pkt *Packet) {
	debug.Println("processing channel data:", ch.chidcli, gopp.StrSuf(pkt.data, 32))
	buf, err := base64.StdEncoding.DecodeString(pkt.data)
	if err != nil {
		errl.Println(err)
	}

	wn, err := ch.conn.Write(buf)
	if err != nil {
		errl.Println(err)
	}
	info.Println("kcp->cli:", wn)
}

//////////////
func (this *Tunnelc) onToxnetSelfConnectionStatus(t *tox.Tox, status uint32, extra unsafe.Pointer) {
	_, err := t.FriendByPublicKey(toxtunid)
	if err != nil {
		t.FriendAdd(toxtunid, "tuncli")
		t.WriteSavedata(fname)
	}
	info.Println(status)
}

func (this *Tunnelc) onToxnetFriendRequest(t *tox.Tox, friendId string, message string, userData unsafe.Pointer) {
	debug.Println(friendId, message)
}

func (this *Tunnelc) onToxnetFriendConnectionStatus(t *tox.Tox, friendNumber uint32, status uint32, userData unsafe.Pointer) {
	fid, _ := this.tox.FriendGetPublicKey(friendNumber)
	info.Println(friendNumber, status, fid)
}

func (this *Tunnelc) onToxnetFriendMessage(t *tox.Tox, friendNumber uint32, message string, userData unsafe.Pointer) {
	debug.Println(friendNumber, len(message), gopp.StrSuf(message, 32))
	buf, err := base64.StdEncoding.DecodeString(message)
	if err != nil {
		errl.Println(err)
	}

	n := this.kcp.Input(buf)
	debug.Println("tox->kcp:", n, len(buf))
}

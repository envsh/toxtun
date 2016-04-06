package main

import (
	"fmt"
	"net"
	// "strings"
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"time"

	"gopp"
	"tox"
)

const (
	server_port = 8113
)

type Tunnelc struct {
	tox    *tox.Tox
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
	t.CallbackFriendLossyPacket(this.onToxnetFriendLossyPacket, nil)

	//
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
	n, err := this.tox.FriendSendMessage(0, string(pkt.toJson()))
	if err != nil {
		// 连接失败
		errl.Println(err)
		this.chpool.rmClient(ch)
		return
	}

	// n := this.kcp.Send(pkt.toJson())
	debug.Println(n, gopp.SubStr(string(pkt.toJson()), 52))
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

		for _, ch := range this.chpool.pool {
			if ch.kcp == nil {
				continue
			}

			ch.kcp.Update(uint32(iclock()))

			n := ch.kcp.Recv(zbuf)
			switch n {
			case -3:
				rbuf := make([]byte, ch.kcp.PeekSize())
				n := ch.kcp.Recv(rbuf)
				go this.processKcpReceive(rbuf, n, ch)
			case -2: // just empty kcp recv queue
				// errl.Println("kcp recv internal error:", n, this.kcp.PeekSize())
			case -1: // EAGAIN
			default:
				errl.Println("unknown recv:", n)
			}
		}
	}
}

func (this *Tunnelc) onKcpOutput(buf []byte, size int, extra interface{}) {
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

func (this *Tunnelc) processKcpReceive(buf []byte, n int, ch *Channel) {
	if len(buf) != n {
		errl.Println("Invalide kcp recv data")
	}

	pkt := parsePacket(buf)
	if pkt.isconnfin() {
		// ch := this.chpool.pool[pkt.chidcli]
		// ch.chidsrv = pkt.chidsrv
		// info.Println("channel connected:", ch.chidcli)
		// go this.processChannel(ch)
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
		sn := ch.kcp.Send(pkt.toJson())
		info.Println("cli->kcp:", sn, ch.conv)
	}
}

func (this *Tunnelc) copyServer2Client(ch *Channel, pkt *Packet) {
	debug.Println("processing channel data:", ch.chidcli, gopp.StrSuf(pkt.data, 52))
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
func (this *Tunnelc) onToxnetSelfConnectionStatus(t *tox.Tox, status uint32, extra interface{}) {
	_, err := t.FriendByPublicKey(toxtunid)
	if err != nil {
		t.FriendAdd(toxtunid, "tuncli")
		t.WriteSavedata(fname)
	}
	info.Println(status)
}

func (this *Tunnelc) onToxnetFriendRequest(t *tox.Tox, friendId string, message string, userData interface{}) {
	debug.Println(friendId, message)
}

func (this *Tunnelc) onToxnetFriendConnectionStatus(t *tox.Tox, friendNumber uint32, status uint32, userData interface{}) {
	fid, _ := this.tox.FriendGetPublicKey(friendNumber)
	info.Println(friendNumber, status, fid)
}

func (this *Tunnelc) onToxnetFriendMessage(t *tox.Tox, friendNumber uint32, message string, userData interface{}) {
	debug.Println(friendNumber, len(message), gopp.StrSuf(message, 52))

	pkt := parsePacket(bytes.NewBufferString(message).Bytes())
	if pkt == nil {
		info.Println("maybe not command, just normal message")
	} else {
		ch := this.chpool.pool[pkt.chidcli]
		ch.conv = pkt.conv
		ch.chidsrv = pkt.chidsrv
		ch.kcp = NewKCP(ch.conv, this.onKcpOutput, ch)
		ch.kcp.SetMtu(tunmtu)
		this.chpool.putClient(ch)

		info.Println("channel connected,", ch.chidcli, ch.chidsrv, ch.conv)
		// can read now，不能阻塞，开新的goroutine
		go this.processChannel(ch)
	}
}

func (this *Tunnelc) onToxnetFriendLossyPacket(t *tox.Tox, friendNumber uint32, message string, userData interface{}) {
	debug.Println(friendNumber, len(message), gopp.StrSuf(message, 52))

	buf := bytes.NewBufferString(message).Bytes()
	if buf[0] == 254 { // lossypacket
		buf = buf[1:]
		var conv uint32
		// kcp包前4字段为conv，little hacky
		if len(buf) < 4 {
			errl.Println("wtf")
		}
		conv = binary.LittleEndian.Uint32(buf)
		ch := this.chpool.pool2[conv]
		if ch == nil {
			info.Println("maybe has some problem")
		}
		n := ch.kcp.Input(buf)
		debug.Println("tox->kcp:", conv, n, len(buf), gopp.StrSuf(string(buf), 52))
	} else {
		info.Println("unknown message:", buf[0])
	}
}

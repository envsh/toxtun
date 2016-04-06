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
	go this.serveKcp()
	iterate(this.tox)
}

///////////
func (this *Tunneld) serveKcp() {
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

func (this *Tunneld) onKcpOutput(buf []byte, size int, extra interface{}) {
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

func (this *Tunneld) processKcpReceive(buf []byte, n int, ch *Channel) {
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
		ch.kcp.Send(repkt.toJson())

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
}

func (this *Tunneld) copyServer2Client(ch *Channel) {
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

		sbuf := base64.StdEncoding.EncodeToString(rbuf[:n])
		pkt := ch.makeDataPacket(sbuf)
		sn := ch.kcp.Send(pkt.toJson())
		info.Println("srv->kcp:", sn)
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
	// toxid+host+port+time
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
		go this.copyServer2Client(ch)
	}
}

func (this *Tunneld) onToxnetFriendLossyPacket(t *tox.Tox, friendNumber uint32, message string, userData interface{}) {
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

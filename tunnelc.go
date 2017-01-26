package main

import (
	"fmt"
	"net"
	// "strings"
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"log"
	"time"

	"github.com/kitech/go-toxcore"
	"gopp"
)

var (
	// TODO dynamic multiple port mode
	// tunnel客户端通道监听服务端口
	tunnelServerPort = 8113
)

type Tunnelc struct {
	tox    *tox.Tox
	srv    net.Listener
	chpool *ChannelPool

	toxPollChan         chan ToxPollEvent
	kcpPollChan         chan KcpPollEvent
	newConnChan         chan NewConnEvent
	clientReadyReadChan chan ClientReadyReadEvent
	clientCloseChan     chan ClientCloseEvent
	clientCheckACKChan  chan ClientCheckACKEvent

	// multipath-udp
}

func NewTunnelc() *Tunnelc {
	this := new(Tunnelc)
	this.chpool = NewChannelPool()

	t := makeTox("toxtunc")
	this.tox = t

	// callbacks
	t.CallbackSelfConnectionStatus(this.onToxnetSelfConnectionStatus, nil)
	// t.CallbackFriendRequest(this.onToxnetFriendRequest, nil)
	t.CallbackFriendConnectionStatus(this.onToxnetFriendConnectionStatus, nil)
	t.CallbackFriendMessage(this.onToxnetFriendMessage, nil)
	// t.CallbackFriendLossyPacket(this.onToxnetFriendLossyPacket, nil)
	// t.CallbackFriendLosslessPacket(this.onToxnetFriendLosslessPacket, nil)

	return this
}

func (this *Tunnelc) serve() {
	tunnelServerPort = config.recs[0].lport
	srv, err := net.Listen("tcp", fmt.Sprintf(":%d", tunnelServerPort))
	if err != nil {
		log.Println(lerrorp, err)
		return
	}
	this.srv = srv
	log.Println(linfop, "tunaddr:", srv.Addr().String())

	mpcsz := 256
	this.toxPollChan = make(chan ToxPollEvent, mpcsz)
	this.kcpPollChan = make(chan KcpPollEvent, mpcsz)
	this.newConnChan = make(chan NewConnEvent, mpcsz)
	this.clientReadyReadChan = make(chan ClientReadyReadEvent, mpcsz)
	this.clientCloseChan = make(chan ClientCloseEvent, mpcsz)
	this.clientCheckACKChan = make(chan ClientCheckACKEvent, 0)

	// install pollers
	go func() {
		for {
			time.Sleep(50 * time.Millisecond)
			this.toxPollChan <- ToxPollEvent{}
			// iterate(this.tox)
		}
	}()

	go this.serveTcp()

	// like event handler
	for {
		select {
		case evt := <-this.newConnChan:
			this.initConnChanel(evt.conn, evt.times, evt.btime)
		case evt := <-this.clientReadyReadChan:
			this.processClientReadyRead(evt.ch, evt.buf, evt.size)
		case evt := <-this.clientCloseChan:
			this.promiseChannelClose(evt.ch)
		case <-this.toxPollChan:
			iterate(this.tox)
		case <-this.kcpPollChan:
			// this.serveKcp()
			panic(123)
		case evt := <-this.clientCheckACKChan:
			this.clientCheckACKRecved(evt.ch)
		}
	}
}

// 手写loop吧，试试
func (this *Tunnelc) pollMain() {

}

func (this *Tunnelc) serveTcp() {
	srv := this.srv

	for {
		c, err := srv.Accept()
		if err != nil {
			log.Println(lerrorp, err)
		}
		// info.Println(c)
		this.newConnChan <- NewConnEvent{c, 0, time.Now()}
		appevt.Trigger("newconn")
	}
}

func (this *Tunnelc) initConnChanel(conn net.Conn, times int, btime time.Time) {
	ch := NewChannelClient(conn)
	ch.ip = config.recs[0].rhost
	ch.port = fmt.Sprintf("%d", config.recs[0].rport)
	this.chpool.putClient(ch)

	toxtunid := config.recs[0].rpubkey
	pkt := ch.makeConnectSYNPacket()
	_, err := this.FriendSendMessage(toxtunid, string(pkt.toJson()))

	if err != nil {
		// 连接失败
		log.Println(ldebugp, err)
		this.chpool.rmClient(ch)
		if times < 10 {
			go func() {
				time.Sleep(500 * time.Millisecond)
				this.newConnChan <- NewConnEvent{conn, times + 1, btime}
			}()
		} else {
			log.Println("connect timeout:", times, time.Now().Sub(btime))
			conn.Close()
			appevt.Trigger("connerr")
		}
		return
	} else {
		go func() {
			time.Sleep(15 * time.Second)
			if _, ok := this.chpool.pool[ch.chidcli]; ok {
				this.clientCheckACKChan <- ClientCheckACKEvent{ch}
			}
		}()
	}
}

func (this *Tunnelc) clientCheckACKRecved(ch *Channel) {

	if _, ok := this.chpool.pool[ch.chidcli]; ok && !ch.conn_ack_recved {
		log.Println("wait connection ack timeout", time.Now().Sub(ch.conn_begin_time))
		this.connectFailedClean(ch)
	}
}

func (this *Tunnelc) connectFailedClean(ch *Channel) {
	this.chpool.rmClient(ch)
	ch.conn.Close()
	appevt.Trigger("connerr")
	ch.addCloseReason("connect_timeout")
	appevt.Trigger("closereason", ch.closeReason())
}

//////////
/*
func (this *Tunnelc) serveKcp() {
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
				this.processKcpReadyRead(ch)
			case -2: // just empty kcp recv queue
			case -1: // EAGAIN
			default:
				errl.Println("unknown recv:", n, ch.chidcli, ch.conv)
			}
		}
	}
}
func (this *Tunnelc) processKcpReadyRead(ch *Channel) {
	buf := make([]byte, ch.kcp.PeekSize())
	n := ch.kcp.Recv(buf)

	if len(buf) != n {
		errl.Println("Invalide kcp recv data")
	}

	pkt := parsePacket(buf)
	if pkt.isdata() {
		ch := this.chpool.pool[pkt.chidcli]
		this.copyServer2Client(ch, pkt)
	} else {
		panic(123)
	}
}

func (this *Tunnelc) onKcpOutput(buf []byte, size int, extra interface{}) {
	if size <= 0 {
		// 如果总是出现，并且不影响程序运行，那么也就不是bug了
		return
	}

	if _, ok := extra.(*Channel); !ok {
	}

	toxtunid := config.recs[0].rpubkey
	msg := string([]byte{254}) + string(buf[:size])
	err := this.FriendSendLossyPacket(toxtunid, msg)
	// msg := string([]byte{191}) + string(buf[:size])
	// err := this.tox.FriendSendLosslessPacket(0, msg)
	if err != nil {
		debug.Println(err)
	} else {
		debug.Println("kcp->tox:", len(msg))
	}

	// multipath-udp backend
}
*/
func (this *Tunnelc) pollClientReadyRead(ch *Channel) {
	// 使用kcp的mtu设置了，这里不再需要限制读取的包大小
	log.Println(ldebugp)
	rbuf := make([]byte, rdbufsz)
	for {
		n, err := ch.conn.Read(rbuf)
		if err != nil {
			log.Println("chan read:", err, ch.chidcli, ch.chidsrv, ch.conv)
			break
		}
		log.Println(linfop, n)

		// 应用层控制kcp.WaitSnd()的大小
		for {
			// if uint32(ch.kcp.WaitSnd()) < ch.kcp.snd_wnd*5 {
			sendbuf := gopp.BytesDup(rbuf[:n])
			// this.processClientReadyRead(ch, rbuf, n)
			this.clientReadyReadChan <- ClientReadyReadEvent{ch, sendbuf, n}
			break
			// } else {
			//time.Sleep(3 * time.Millisecond)
			// }
		}
	}

	// 连接结束
	log.Println(ldebugp, "connection closed, cleaning up...:", ch.chidcli, ch.chidsrv, ch.conv)
	ch.client_socket_close = true
	this.clientCloseChan <- ClientCloseEvent{ch}
	appevt.Trigger("connact", -1)
}

func (this *Tunnelc) promiseChannelClose(ch *Channel) {
	log.Println("cleaning up:", ch.chidcli, ch.chidsrv, ch.conv)
	toxtunid := config.recs[0].rpubkey
	if ch.client_socket_close == true && ch.server_socket_close == false {
		pkt := ch.makeCloseFINPacket()
		_, err := this.FriendSendMessage(toxtunid, string(pkt.toJson()))
		if err != nil {
			// 连接失败
			log.Println(err, ch.chidcli, ch.chidsrv, ch.conv)
			return
		}
		ch.addCloseReason("client_close")
		log.Println("client socket closed, notify server.", ch.chidcli, ch.chidsrv, ch.conv, ch.closeReason())
		this.chpool.rmClient(ch)
		appevt.Trigger("closereason", ch.closeReason())
	} else if ch.client_socket_close == true && ch.server_socket_close == true {
		//
		ch.addCloseReason("both_close")
		log.Println("both socket closed:", ch.chidcli, ch.chidsrv, ch.conv, ch.closeReason())
		this.chpool.rmClient(ch)
		appevt.Trigger("closereason", ch.closeReason())
	} else if ch.client_socket_close == false && ch.server_socket_close == true {
		ch.addCloseReason("server_close")
		log.Println("server socket closed, force close client", ch.chidcli, ch.chidsrv, ch.conv, ch.closeReason())
		ch.client_socket_close = true // ch.conn真正关闭可能有延时，造成此处重复处理。提前设置关闭标识。
		ch.conn.Close()
		appevt.Trigger("closereason", ch.closeReason())
	} else {
		log.Println("what state:", ch.chidcli, ch.chidsrv, ch.conv,
			ch.server_socket_close, ch.server_kcp_close, ch.client_socket_close)
		panic("Ooops")
	}
}

func (this *Tunnelc) processClientReadyRead(ch *Channel, buf []byte, size int) {
	sbuf := base64.StdEncoding.EncodeToString(buf[:size])
	pkt := ch.makeDataPacket(sbuf)
	// sn := ch.kcp.Send(pkt.toJson())
	log.Println(ldebugp)
	ch.tp.sendData(string(pkt.toJson()), "")
	sn := 0
	log.Println(ldebugp, "cli->kcp:", size, sn, ch.conv)
	appevt.Trigger("reqbytes", size, len(pkt.toJson())+25)
}

func (this *Tunnelc) copyServer2Client(ch *Channel, pkt *Packet) {
	log.Println(ldebugp, "processing channel data:", ch.chidcli, gopp.StrSuf(pkt.data, 52))
	buf, err := base64.StdEncoding.DecodeString(pkt.data)
	if err != nil {
		log.Println(lerrorp, err)
	}

	wn, err := ch.conn.Write(buf)
	if err != nil {
		log.Println(ldebugp, err)
	} else {
		log.Println(ldebugp, "kcp->cli:", wn)
		appevt.Trigger("respbytes", wn, len(pkt.data)+25)
	}
}

//////////////
func (this *Tunnelc) onToxnetSelfConnectionStatus(t *tox.Tox, status int, extra interface{}) {
	toxtunid := config.recs[0].rpubkey
	_, err := t.FriendByPublicKey(toxtunid)
	if err != nil {
		t.FriendAdd(toxtunid, "tuncli")
		t.WriteSavedata(fname)
	}
	log.Println("mytox status:", status)
	if status == 0 {
		switchServer(t)
	}

	if status == 0 {
		appevt.Trigger("selfonline", false)
		appevt.Trigger("selfoffline")
	} else {
		appevt.Trigger("selfonline", true)
	}
}

func (this *Tunnelc) onToxnetFriendRequest(t *tox.Tox, friendId string, message string, userData interface{}) {
	log.Println(ldebugp, friendId, message)
}

func (this *Tunnelc) onToxnetFriendConnectionStatus(t *tox.Tox, friendNumber uint32, status int, userData interface{}) {
	fid, _ := this.tox.FriendGetPublicKey(friendNumber)
	log.Println("peer status (fn/st/id):", friendNumber, status, fid)
	if status == 0 {
		if friendInConfig(fid) {
			switchServer(t)
		}
	}

	if status == 0 {
		appevt.Trigger("peeronline", false)
		appevt.Trigger("peeroffline")
	} else {
		appevt.Trigger("peeronline", true)
	}
}

func (this *Tunnelc) onToxnetFriendMessage(t *tox.Tox, friendNumber uint32, message string, userData interface{}) {
	log.Println(ldebugp, friendNumber, len(message), gopp.StrSuf(message, 52))
	pkt := parsePacket(bytes.NewBufferString(message).Bytes())
	if pkt == nil {
		log.Println("maybe not command, just normal message")
	} else {
		if pkt.command == CMDCONNACK {
			if ch, ok := this.chpool.pool[pkt.chidcli]; ok {
				ch.conv = pkt.conv
				ch.chidsrv = pkt.chidsrv
				ch.toxid = config.recs[0].rpubkey
				ch.tp = NewKcpTransport(this.tox, ch, false)
				/*
					ch.kcp = NewKCP(ch.conv, this.onKcpOutput, ch)
					ch.kcp.SetMtu(tunmtu)
					if kcp_mode == "fast" {
						ch.kcp.WndSize(128, 128)
						ch.kcp.NoDelay(1, 10, 2, 1)
					}
				*/
				this.chpool.putClientLacks(ch)

				// send ping to udp
				/*
					uaddr, err := net.ResolveUDPAddr("udp", outboundip)
					if err != nil {
						errl.Println(err, uaddr)
					}
					ch.udp_peer_addr = uaddr
					uaddr, err = net.ResolveUDPAddr("udp", pkt.data)
					if err != nil {
						errl.Println(err)
					} else {
						ch.udp_peer_addr = uaddr
					}
				*/
				// wrn, err := this.udpPeer.WriteTo([]byte("hehehheheh"), uaddr)
				// info.Println(wrn, err)

				log.Println("channel connected,", ch.chidcli, ch.chidsrv, ch.conv, pkt.data)
				appevt.Trigger("connok")
				appevt.Trigger("connact", 1)
				ch.conn_ack_recved = true
				// can read now，不能阻塞，开新的goroutine
				go this.pollClientReadyRead(ch)
			} else {
				log.Println("maybe conn ack response timeout", pkt.chidcli, pkt.chidsrv, pkt.conv)
				// TODO 应该给服务器回个关闭包
				ch := NewChannelFromPacket(pkt)
				newpkt := ch.makeCloseFINPacket()
				this.tox.FriendSendMessage(friendNumber, string(newpkt.toJson()))
			}
		} else if pkt.command == CMDCLOSEFIN {
			if ch, ok := this.chpool.pool2[pkt.conv]; ok {
				ch.server_socket_close = true
				this.promiseChannelClose(ch)
			} else if ch, ok := this.chpool.pool[pkt.chidcli]; ok {
				log.Println("maybe server connection failed",
					pkt.command, pkt.chidcli, pkt.chidsrv, pkt.conv)
				// this.connectFailedClean(ch)
				this.promiseChannelClose(ch)
			} else {
				log.Println("recv server close, but maybe client already closed",
					pkt.command, pkt.chidcli, pkt.chidsrv, pkt.conv)
			}
		} else {
			log.Println(lerrorp, "wtf, unknown cmmand:", pkt.command, pkt.chidcli, pkt.chidsrv, pkt.conv)
		}
	}
}

func (this *Tunnelc) onToxnetFriendLossyPacket(t *tox.Tox, friendNumber uint32, message string, userData interface{}) {
	log.Println(ldebugp, friendNumber, len(message), gopp.StrSuf(message, 52))
	buf := bytes.NewBufferString(message).Bytes()
	if buf[0] == 254 { // lossypacket
		buf = buf[1:]
		var conv uint32
		// kcp包前4字段为conv，little hacky
		if len(buf) < 4 {
			log.Println(lerrorp, "wtf")
		}
		conv = binary.LittleEndian.Uint32(buf)
		ch := this.chpool.pool2[conv]
		if ch == nil {
			log.Println("channel not found, maybe has some problem, maybe already closed", conv)
			// TODO 应该给服务器回个关闭包
			// TODO 这个地方发送的包容易出现重复，但是需要服务端处理
			pkt := NewBrokenPacket(conv)
			ch := NewChannelFromPacket(pkt)
			newpkt := ch.makeCloseFINPacket()
			this.tox.FriendSendMessage(friendNumber, string(newpkt.toJson()))
		} else {
			// n := ch.kcp.Input(buf)
			// debug.Println("tox->kcp:", conv, n, len(buf), gopp.StrSuf(string(buf), 52))
			panic(123)
		}
	} else {
		log.Println("unknown message:", buf[0])
	}
}

func (this *Tunnelc) onToxnetFriendLosslessPacket(t *tox.Tox, friendNumber uint32, message string, userData interface{}) {
	log.Println(ldebugp, friendNumber, len(message), gopp.StrSuf(message, 52))
	buf := bytes.NewBufferString(message).Bytes()
	if buf[0] == 191 { // lossypacket
		buf = buf[1:]
		var conv uint32
		// kcp包前4字段为conv，little hacky
		if len(buf) < 4 {
			log.Println(lerrorp, "wtf")
		}
		conv = binary.LittleEndian.Uint32(buf)
		ch := this.chpool.pool2[conv]
		if ch == nil {
			log.Println(lerrorp, "maybe has some problem")
		}
		// n := ch.kcp.Input(buf)
		// debug.Println("tox->kcp:", conv, n, len(buf), gopp.StrSuf(string(buf), 52))
		panic(123)
	} else {
		log.Println("unknown message:", buf[0])
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

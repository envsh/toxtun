package main

import (
	"context"
	"fmt"
	"log"
	"net"

	// "strings"
	"bytes"
	"encoding/binary"
	"time"

	"mkuse/appcm"
	"mkuse/rudp"

	tox "github.com/TokTok/go-toxcore-c"
	"github.com/envsh/go-toxcore/mintox"
	"github.com/kitech/goplusplus"
	"golang.org/x/time/rate"
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
	tox     *tox.Tox
	mtox    *MTox
	usemtox bool
	srvs    map[string]*TunListener
	chpool  *ChannelPool

	toxPollChan chan ToxPollEvent
	// toxReadyReadChan    chan ToxReadyReadEvent
	// toxMessageChan      chan ToxMessageEvent
	kcpPollChan chan KcpPollEvent
	// kcpReadyReadChan    chan KcpReadyReadEvent
	// kcpOutputChan       chan KcpOutputEvent
	kcpInputChan        chan ClientReadyReadEvent
	newConnChan         chan NewConnEvent
	clientReadyReadChan chan ClientReadyReadEvent
	clientCloseChan     chan ClientCloseEvent
	clientCheckACKChan  chan ClientCheckACKEvent
}

func NewTunnelc() *Tunnelc {
	this := new(Tunnelc)
	this.chpool = NewChannelPool()
	this.srvs = make(map[string]*TunListener)

	t := makeTox("toxtunc")
	this.tox = t
	this.usemtox = true
	this.mtox = newMinTox("toxtunc")

	// callbacks
	this.mtox.DataFunc = this.onMinToxData

	// callbacks
	t.CallbackSelfConnectionStatus(this.onToxnetSelfConnectionStatus, nil)
	t.CallbackFriendRequest(this.onToxnetFriendRequest, nil)
	t.CallbackFriendConnectionStatus(this.onToxnetFriendConnectionStatus, nil)
	t.CallbackFriendMessage(this.onToxnetFriendMessage, nil)
	t.CallbackFriendLossyPacket(this.onToxnetFriendLossyPacket, nil)
	t.CallbackFriendLosslessPacket(this.onToxnetFriendLosslessPacket, nil)

	//
	return this
}

func (this *Tunnelc) serve() {
	recs := config.recs
	this.tox.SelfSetStatusMessage(fmt.Sprintf("%s of toxtun, %+v", "toxtuncs", recs))
	this.listenTunnels()

	this.toxPollChan = make(chan ToxPollEvent, mpcsz)
	// this.toxReadyReadChan = make(chan ToxReadyReadEvent, 0)
	// this.toxMessageChan = make(chan ToxMessageEvent, 0)
	this.kcpPollChan = make(chan KcpPollEvent, mpcsz)
	// this.kcpReadyReadChan = make(chan KcpReadyReadEvent, 0)
	// this.kcpOutputChan = make(chan KcpOutputEvent, 0)
	this.kcpInputChan = make(chan ClientReadyReadEvent, mpcsz)
	this.newConnChan = make(chan NewConnEvent, mpcsz)
	this.clientReadyReadChan = make(chan ClientReadyReadEvent, mpcsz)
	this.clientCloseChan = make(chan ClientCloseEvent, mpcsz)
	this.clientCheckACKChan = make(chan ClientCheckACKEvent, 0)

	// install pollers
	go func() {
		for {
			time.Sleep(time.Duration(smuse.tox_interval) * time.Millisecond)
			// time.Sleep(200 * time.Millisecond)
			this.toxPollChan <- ToxPollEvent{}
			// iterate(this.tox)
		}
	}()
	go func() {
		for {
			time.Sleep(time.Duration(smuse.kcp_interval) * time.Millisecond)
			// time.Sleep(200 * time.Millisecond)
			this.kcpPollChan <- KcpPollEvent{}
			// this.serveKcp()
		}
	}()
	this.serveTunnels()

	// like event handler
	for {
		select {
		// case evt := <-this.toxReadyReadChan:
		// 	this.processFriendLossyPacket(this.tox, evt.friendNumber, evt.message, nil)
		// case evt := <-this.toxMessageChan:
		// 	this.processFriendMessage(this.tox, evt.friendNumber, evt.message, nil)
		// case evt := <-this.kcpReadyReadChan:
		// 	this.processKcpReadyRead(evt.ch)
		// case evt := <-this.kcpOutputChan:
		// 	this.processKcpOutput(evt.buf, evt.size, evt.extra)
		case evt := <-this.kcpInputChan:
			// evt.ch.kcp.Input(evt.buf, true, true)
			evt.ch.rudp_.Input(evt.buf)
		case evt := <-this.newConnChan:
			this.initConnChannel(evt.conn, evt.times, evt.btime, evt.tname)
		case evt := <-this.clientReadyReadChan:
			this.processClientReadyRead(evt.ch, evt.buf, evt.size)
		case evt := <-this.clientCloseChan:
			this.promiseChannelClose(evt.ch)
		case <-this.toxPollChan:
			if !this.usemtox {
				iterate(this.tox)
			}
		case <-this.kcpPollChan:
			this.serveKcp()
		case evt := <-this.clientCheckACKChan:
			this.clientCheckACKRecved(evt.ch)
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
			this.mtox.addFriend(tunrec.rpubkey)
		} else if tunrec.tproto == "udp" {
			srv, err := ListenUDP(fmt.Sprintf(":%d", tunnelServerPort))
			if err != nil {
				log.Println(lerrorp, err, tname, tunnelServerPort)
				continue
			}
			this.srvs[tunrec.tname] = newTunListenerUdp2(srv)
			this.mtox.addFriend(tunrec.rpubkey)
		} else {
			log.Panicln("wtf,", tunrec)
		}
		log.Println(linfop, fmt.Sprintf("#T%s", tname), "tunaddr:", tunrec.tname, tunrec.tproto, tunrec.lhost, tunrec.lport)
	}
}

// 手写loop吧，试试
func (this *Tunnelc) pollMain() {

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
		info.Println("New connection from/to:", c.RemoteAddr(), c.LocalAddr(), tname)
		this.newConnChan <- NewConnEvent{c, 0, time.Now(), tname}
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
		info.Println("New connection from/to:", c.RemoteAddr(), c.LocalAddr(), tname)
		this.newConnChan <- NewConnEvent{c, 0, time.Now(), tname}
		appevt.Trigger("newconn", tname)
	}
}

func (this *Tunnelc) initConnChannel(conn net.Conn, times int, btime time.Time, tname string) {
	ch := NewChannelClient(conn, tname)
	// ch.ip = "127.0.0.1"
	// ch.port = "8118"
	tunrec := config.getRecordByName(tname)
	ch.ip = tunrec.rhost
	ch.port = fmt.Sprintf("%d", tunrec.rport)
	ch.conv = makeKcpConv(this.tox.SelfGetPublicKey(), ch.ip, ch.port+gopp.RandomNumber(8))
	this.chpool.putClient(ch)

	toxtunid := tunrec.rpubkey
	pkt := ch.makeConnectSYNPacket()
	var err error
	if !this.usemtox {
		_, err = this.FriendSendMessage(toxtunid, string(pkt.toJson()))
	} else {
		err = this.mtox.sendData(pkt.toJson(), true, true)
	}

	if err != nil {
		// 连接失败
		debug.Println(err, tname)
		this.chpool.rmClient(ch)
		if times < 10 {
			go func() {
				time.Sleep(500 * time.Millisecond)
				this.newConnChan <- NewConnEvent{conn, times + 1, btime, tname}
			}()
		} else {
			info.Println("connect timeout:", times, time.Now().Sub(btime), tname)
			conn.Close()
			appevt.Trigger("connerr", tname)
		}
		return
	} else {
		// ch.conv = pkt.Conv
		ch.chidsrv = pkt.Chidsrv
		// ch.kcp = NewKCP(ch.conv, this.onKcpOutput, ch)
		// ch.kcp.SetMtu(tunmtu) // TODO for tcp, little bigger
		// ch.kcp.WndSize(smuse.wndsz, smuse.wndsz)
		// ch.kcp.NoDelay(smuse.nodelay, smuse.interval, smuse.resend, smuse.nc)
		ch.rudp_ = rudp.NewRUDP(ch.conv, func(data []byte, prior bool) error {
			return this.onKcpOutput2(data, len(data), ch, prior)
		})
		// ch.fp, _ = os.OpenFile(fmt.Sprintf("convc%d", ch.conv), os.O_RDWR|os.O_CREATE, 0644)
		this.chpool.putClientLacks(ch)
		// can read now，不能阻塞，开新的goroutine
		// go this.pollClientReadyRead(ch)
		go this.pollServerReadyRead(ch)

		time.AfterFunc(15*time.Second, func() {
			if ch := this.chpool.getPool1ById(ch.chidcli); ch != nil {
				this.clientCheckACKChan <- ClientCheckACKEvent{ch}
			}
		})
	}
}

func (this *Tunnelc) initConnChannel_dep(conn net.Conn, times int, btime time.Time, tname string) {
	ch := NewChannelClient(conn, tname)
	// ch.ip = "127.0.0.1"
	// ch.port = "8118"
	tunrec := config.getRecordByName(tname)
	ch.ip = tunrec.rhost
	ch.port = fmt.Sprintf("%d", tunrec.rport)
	ch.conv = makeKcpConv(this.tox.SelfGetPublicKey(), ch.ip, ch.port+gopp.RandomNumber(8))
	this.chpool.putClient(ch)

	toxtunid := tunrec.rpubkey
	pkt := ch.makeConnectSYNPacket()
	var err error
	if this.usemtox {
		_, err = this.FriendSendMessage(toxtunid, string(pkt.toJson()))
	} else {
		err = this.mtox.sendData(pkt.toJson(), true, true)
		gopp.ErrPrint(err)
	}

	if err != nil {
		// 连接失败
		debug.Println(err, tname)
		this.chpool.rmClient(ch)
		if times < 10 {
			go func() {
				time.Sleep(500 * time.Millisecond)
				this.newConnChan <- NewConnEvent{conn, times + 1, btime, tname}
			}()
		} else {
			info.Println("connect timeout:", times, time.Now().Sub(btime), tname)
			conn.Close()
			appevt.Trigger("connerr", tname)
		}
		return
	} else {
		// ch.conv = pkt.Conv
		ch.chidsrv = pkt.Chidsrv
		ch.kcp = NewKCP(ch.conv, this.onKcpOutput, ch)
		ch.kcp.SetMtu(tunmtu) // TODO for tcp, little bigger
		ch.kcp.WndSize(smuse.wndsz, smuse.wndsz)
		ch.kcp.NoDelay(smuse.nodelay, smuse.interval, smuse.resend, smuse.nc)
		this.chpool.putClientLacks(ch)
		// can read now，不能阻塞，开新的goroutine
		go this.pollClientReadyRead(ch)

		time.AfterFunc(15*time.Second, func() {
			if ch := this.chpool.getPool1ById(ch.chidcli); ch != nil {
				this.clientCheckACKChan <- ClientCheckACKEvent{ch}
			}
		})
	}
}

func (this *Tunnelc) clientCheckACKRecved(ch *Channel) {

	if ch_ := this.chpool.getPool1ById(ch.chidcli); ch_ != nil && !ch.conn_ack_recved {
		info.Println("wait connection ack timeout", time.Now().Sub(ch.conn_begin_time))
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
func (this *Tunnelc) serveKcp() {
	zbuf := make([]byte, 0)
	if true {
		for _, ch := range this.chpool.pool {
			if ch.kcp == nil {
				continue
			}

			ch.kcp.Update()

			n := ch.kcp.Recv(zbuf)
			switch n {
			case -3:
				this.processKcpReadyRead(ch)
			case -2: // just empty kcp recv queue
			case -1: // EAGAIN
			default:
				errl.Println("unknown recv:", n, ch.chidcli, ch.conv)
			}
			if n != -3 { // check read timeout for client
				// 一般浏览器超时时间120s
				dtime := int(time.Since(ch.last_net_recv).Seconds())
				if dtime > 1800 && ch.last_net_recv.Nanosecond() > ch.conn_begin_time.Nanosecond() {
					info.Println("read timeout from kcp(remnet)", dtime, ch.conv, ch.conn_begin_time)
					// TODO 有待更多测试
					ch.addCloseReason("read timeout from kcp(remnet)")
					ch.conn.Close()
				}
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
		ch := this.chpool.getPool1ById(pkt.Chidcli)
		this.copyServer2Client(ch, pkt)
		ch.last_net_recv = time.Now()
	} else {
		panic(123)
	}
}

func (this *Tunnelc) onKcpOutput2(buf []byte, size int, extra interface{}, prior bool) error {
	if size <= 0 {
		// 如果总是出现，并且不影响程序运行，那么也就不是bug了
		return fmt.Errorf("invalid size %d", size)
	}

	ch, ok := extra.(*Channel)
	if !ok {
		errl.Println("extra is not a *Channel:", extra)
		return fmt.Errorf("extra is not a *Channel")
	}

	tunrec := config.getRecordByName(ch.tname)
	toxtunid := tunrec.rpubkey

	var sndlen int = size
	var err error
	if !this.usemtox {
		sndlen += 1
		msg := string([]byte{254}) + string(buf[:size])
		err = this.FriendSendLossyPacket(toxtunid, msg)
		// msg := string([]byte{191}) + string(buf[:size])
		// err := this.tox.FriendSendLosslessPacket(0, msg)
	} else {
		err = this.mtox.sendData(buf[:size], false, prior)
	}
	if err != nil {
		debug.Println(err)
	} else {
		debug.Println("kcp->tox:", sndlen)
	}
	return err
}

func (this *Tunnelc) onKcpOutput(buf []byte, size int, extra interface{}) {
	this.onKcpOutput2(buf, size, extra, false)
}

func (this *Tunnelc) pollClientReadyRead(ch *Channel) {
	lmter := rate.NewLimiter(rate.Limit(1024*1024*3/2), 1024*1024*2)
	// 使用kcp的mtu设置了，这里不再需要限制读取的包大小

	for {
		rbuf := make([]byte, rdbufsz)
		n, err := ch.conn.Read(rbuf)
		if err != nil {
			info.Println("chan sock read:", err, ch.chidcli, ch.chidsrv, ch.conv)
			break
		}

		if false {
			lmter.WaitN(context.Background(), n)
			// sendbuf := gopp.BytesDup(rbuf[:n])
			// this.clientReadyReadChan <- ClientReadyReadEvent{ch, sendbuf, n, false}
			// continue
		}
		wn, err := ch.rudp_.Write(rbuf[:n])
		gopp.ErrPrint(err, wn)
		if err != nil {
			break
		}

		// 应用层控制kcp.WaitSnd()的大小
		for false {
			// info.Printf("chan WaitSnd:%d, snd_wnd:%d, snd_wnd*3:%d, snd_buf:%d",
			//	ch.kcp.WaitSnd(), ch.kcp.snd_wnd, ch.kcp.snd_wnd*3, len(ch.kcp.snd_buf))
			if uint32(ch.kcp.WaitSnd()) < ch.kcp.snd_wnd*8 {
				sendbuf := gopp.BytesDup(rbuf[:n])
				// this.processClientReadyRead(ch, rbuf, n)
				this.clientReadyReadChan <- ClientReadyReadEvent{ch, sendbuf, n, false}
				break
			} else {
				time.Sleep(3 * time.Millisecond)
			}
		}
	}

	// 连接结束
	debug.Println("connection closed, cleaning up...:", ch.chidcli, ch.chidsrv, ch.conv)
	ch.client_socket_close = true
	this.clientCloseChan <- ClientCloseEvent{ch}
	appevt.Trigger("connact", -1)
}

func (this *Tunnelc) pollServerReadyRead(ch *Channel) {
	lmter := rate.NewLimiter(rate.Limit(1024*1024*3/2), 1024*1024*2)
	// 使用kcp的mtu设置了，这里不再需要限制读取的包大小

	for {
		rbuf := make([]byte, rdbufsz)
		rn, err := ch.rudp_.Read(rbuf)
		if err != nil {
			info.Println("chan sock read:", err, ch.chidcli, ch.chidsrv, ch.conv)
			break
		}

		if false {
			lmter.WaitN(context.Background(), rn)
			// sendbuf := gopp.BytesDup(rbuf[:n])
			// this.clientReadyReadChan <- ClientReadyReadEvent{ch, sendbuf, rn, false}
			// continue
		}
		wn, err := ch.conn.Write(rbuf[:rn])
		gopp.ErrPrint(err, wn)
		// ch.fp.Write(rbuf[:rn])
		if err != nil {
			break
		}
		log.Println("rudp -> cli2", rn)

		// 应用层控制kcp.WaitSnd()的大小
		for false {
			// info.Printf("chan WaitSnd:%d, snd_wnd:%d, snd_wnd*3:%d, snd_buf:%d",
			//	ch.kcp.WaitSnd(), ch.kcp.snd_wnd, ch.kcp.snd_wnd*3, len(ch.kcp.snd_buf))
			if uint32(ch.kcp.WaitSnd()) < ch.kcp.snd_wnd*8 {
				sendbuf := gopp.BytesDup(rbuf[:rn])
				// this.processClientReadyRead(ch, rbuf, rn)
				this.clientReadyReadChan <- ClientReadyReadEvent{ch, sendbuf, rn, false}
				break
			} else {
				time.Sleep(3 * time.Millisecond)
			}
		}
	}

	// 连接结束
	debug.Println("connection closed, cleaning up...:", ch.chidcli, ch.chidsrv, ch.conv)
	ch.client_socket_close = true
	this.clientCloseChan <- ClientCloseEvent{ch}
	appevt.Trigger("connact", -1)
}

func (this *Tunnelc) promiseChannelClose(ch *Channel) {
	info.Println("cleaning up:", ch.chidcli, ch.chidsrv, ch.conv, ch.tname)
	tunrec := config.getRecordByName(ch.tname)
	toxtunid := tunrec.rpubkey
	if ch.client_socket_close == true && ch.server_socket_close == false {
		pkt := ch.makeCloseFINPacket()
		var err error
		if !this.usemtox {
			_, err = this.FriendSendMessage(toxtunid, string(pkt.toJson()))
		} else {
			err = this.mtox.sendData(pkt.toJson(), true, true)
		}
		if err != nil {
			// 连接失败
			info.Println(err, ch.chidcli, ch.chidsrv, ch.conv)
			return
		}
		ch.addCloseReason("client_close")
		info.Println("client socket closed, notify server.", ch.chidcli, ch.chidsrv, ch.conv, ch.closeReason())
		ch.rudp_.Close()
		this.chpool.rmClient(ch)
		appevt.Trigger("closereason", ch.closeReason())
	} else if ch.client_socket_close == true && ch.server_socket_close == true {
		//
		ch.addCloseReason("both_close")
		info.Println("both socket closed:", ch.chidcli, ch.chidsrv, ch.conv, ch.closeReason())
		ch.rudp_.Close()
		this.chpool.rmClient(ch)
		appevt.Trigger("closereason", ch.closeReason())
	} else if ch.client_socket_close == false && ch.server_socket_close == true {
		ch.addCloseReason("server_close")
		info.Println("server socket closed, force close client", ch.chidcli, ch.chidsrv, ch.conv, ch.closeReason())
		ch.client_socket_close = true // ch.conn真正关闭可能有延时，造成此处重复处理。提前设置关闭标识。
		ch.conn.Close()
		ch.rudp_.Close()
		appevt.Trigger("closereason", ch.closeReason())
	} else {
		info.Println("what state:", ch.chidcli, ch.chidsrv, ch.conv,
			ch.server_socket_close, ch.server_kcp_close, ch.client_socket_close)
		panic("Ooops")
	}
}

func (this *Tunnelc) processClientReadyRead(ch *Channel, buf []byte, size int) {
	sbuf := buf
	pkt := ch.makeDataPacket(sbuf)
	sn := ch.kcp.Send(pkt.toJson())
	appcm.Meter("tunc.cli2kcp.len.total").Mark(int64(size))
	debug.Println("cli->kcp:", sn, ch.conv)
	appevt.Trigger("reqbytes", size, len(pkt.toJson())+25)
}

func (this *Tunnelc) copyServer2Client(ch *Channel, pkt *Packet) {
	debug.Println("processing channel data:", ch.chidcli, gopp.StrSuf(string(pkt.Data), 52))

	buf := pkt.Data
	wn, err := ch.conn.Write(buf)
	if err != nil {
		debug.Println(err)
	} else {
		appcm.Meter("tunc.kcp2cli.len.total").Mark(int64(wn))
		debug.Println("kcp->cli:", wn)
		appevt.Trigger("respbytes", wn, len(pkt.Data)+25)
	}
}

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
		if !this.usemtox {
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

// TODO 应该把数据发送到chan进入主循环再回来处理，就不会有concurrent问题
func (this *Tunnelc) onMinToxData(data []byte, cbdata mintox.Object, ctrl bool) {
	debug.Println(len(data), ctrl)

	if ctrl {
		message := string(data)
		pkt := parsePacket(bytes.NewBufferString(message).Bytes())
		this.handleCtrlPacket(pkt, 0)
	} else {
		this.handleDataPacket(data, 0)
	}
}
func (this *Tunnelc) handleCtrlPacket(pkt *Packet, friendNumber uint32) {
	if pkt.Command == CMDCONNACK {
		if ch := this.chpool.getPool1ById(pkt.Chidcli); ch != nil {
			// ch.conv = pkt.Conv
			// ch.chidsrv = pkt.Chidsrv
			// ch.kcp = NewKCP(ch.conv, this.onKcpOutput, ch)
			// ch.kcp.SetMtu(tunmtu) // TODO for tcp, little bigger
			// ch.kcp.WndSize(smuse.wndsz, smuse.wndsz)
			// ch.kcp.NoDelay(smuse.nodelay, smuse.interval, smuse.resend, smuse.nc)
			// this.chpool.putClientLacks(ch)

			info.Println("channel connected,", ch.chidcli, ch.chidsrv, ch.conv, pkt.Data)
			appevt.Trigger("connok")
			appevt.Trigger("connact", 1)
			ch.conn_ack_recved = true
			// can read now，不能阻塞，开新的goroutine
			go this.pollClientReadyRead(ch)
		} else {
			info.Println("maybe conn ack response timeout", pkt.Chidcli, pkt.Chidsrv, pkt.Conv)
			// TODO 应该给服务器回个关闭包
			ch := NewChannelFromPacket(pkt)
			newpkt := ch.makeCloseFINPacket()
			if !this.usemtox {
				this.tox.FriendSendMessage(friendNumber, string(newpkt.toJson()))
			} else {
				this.mtox.sendData(newpkt.toJson(), true, true)
			}
		}
	} else if pkt.Command == CMDCLOSEFIN {
		ch2 := this.chpool.getPool2ById(pkt.Conv)
		ch1 := this.chpool.getPool1ById(pkt.Chidcli)
		if ch2 != nil {
			ch2.server_socket_close = true
			this.promiseChannelClose(ch2)
		} else if ch1 != nil {
			info.Println("maybe server connection failed", pkt.Command, pkt.Chidcli, pkt.Chidsrv, pkt.Conv)
			// this.connectFailedClean(ch)
			this.promiseChannelClose(ch1)
		} else {
			info.Println("recv server close, but maybe client already closed",
				pkt.Command, pkt.Chidcli, pkt.Chidsrv, pkt.Conv)
		}
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
	ch := this.chpool.getPool2ById(conv) // should be fixed by mutex
	if ch == nil {
		info.Println("channel not found, maybe has some problem, maybe already closed", conv)
		// TODO 应该给服务器回个关闭包
		// TODO 这个地方发送的包容易出现重复，但是需要服务端处理
		pkt := NewBrokenPacket(conv)
		ch := NewChannelFromPacket(pkt)
		newpkt := ch.makeCloseFINPacket()
		if !this.usemtox {
			_, err := this.tox.FriendSendMessage(friendNumber, string(newpkt.toJson()))
			gopp.ErrPrint(err)
		} else {
			err := this.mtox.sendData(newpkt.toJson(), true, true)
			gopp.ErrPrint(err)
		}
	} else {
		this.kcpInputChan <- ClientReadyReadEvent{ch, buf, len(buf), false}
		// n := ch.kcp.Input(buf, true, true)
		n := len(buf)
		debug.Println("tox->kcp:", conv, n, len(buf), gopp.StrSuf(string(buf), 52))
	}
}

func (this *Tunnelc) onToxnetFriendMessage(t *tox.Tox, friendNumber uint32, message string, userData interface{}) {
	debug.Println(friendNumber, len(message), gopp.StrSuf(message, 52))
	pkt := parsePacket(bytes.NewBufferString(message).Bytes())
	if pkt == nil {
		info.Println("maybe not command, just normal message:", gopp.StrSuf(message, 52))
	} else {
		this.handleCtrlPacket(pkt, friendNumber)
	}
}

func (this *Tunnelc) onToxnetFriendLossyPacket(t *tox.Tox, friendNumber uint32, message string, userData interface{}) {
	debug.Println(friendNumber, len(message), gopp.StrSuf(message, 52))
	buf := bytes.NewBufferString(message).Bytes()
	if buf[0] == 254 { // lossypacket
		this.handleDataPacket(buf[1:], friendNumber)
	} else {
		info.Println("unknown message:", buf[0])
	}
}

func (this *Tunnelc) onToxnetFriendLosslessPacket(t *tox.Tox, friendNumber uint32, message string, userData interface{}) {
	debug.Println(friendNumber, len(message), gopp.StrSuf(message, 52))
	buf := bytes.NewBufferString(message).Bytes()
	if buf[0] == 191 { // lossypacket
		buf = buf[1:]
		var conv uint32
		// kcp包前4字节为conv，little hacky
		if len(buf) < 4 {
			errl.Println("wtf")
		}
		conv = binary.LittleEndian.Uint32(buf)
		ch := this.chpool.getPool2ById(conv)
		if ch == nil {
			errl.Println("maybe has some problem")
		}
		n := ch.kcp.Input(buf, true, true)
		debug.Println("tox->kcp:", conv, n, len(buf), gopp.StrSuf(string(buf), 52))
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

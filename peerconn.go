package main

import (
	"fmt"
	"gopp"
	"io"
	"log"
	"net"
	"net/url"
	"sync"
	"time"

	rudp "mkuse/rudp2"

	tox "github.com/TokTok/go-toxcore-c"
)

type PeerConnPool struct {
	conns sync.Map // friendNumber | public_key => *PeerConn
}

type PeerConn struct {
	tox      *tox.Tox
	peerno   uint32
	tptype   int
	mtvtp    *rudp.Vtpconn
	muxlsner MuxListener
	muxsess  MuxSession

	// client
	newConnC chan NewConnEvent
	// muxConnedC    chan bool
	// muxDisconnedC chan bool
	// newconnq      []NewConnEvent // 连接阶段暂存连接请求
}

func NewPeerConn(tox *tox.Tox, peerno uint32, tptype int) *PeerConn {
	this := &PeerConn{}
	this.tox = tox
	this.peerno = peerno
	this.tptype = tptype

	writeout := this.writeout4tox
	this.mtvtp = rudp.NewVtpconn(writeout)
	// this.muxlsner = RudpListen(this.mtvtp)
	this.muxlsner = NewRudp3Listener(this.mtvtp)

	go this.servesrv()
	go this.servecli()

	return this
}

func (this *PeerConn) GetInputer() Inputer { return this.mtvtp }

func (this *PeerConn) servesrv() {
	// like event handler
	for {
		sess, err := this.muxlsner.Accept()
		gopp.ErrPrint(err)
		if err != nil {
			break
		}

		log.Println("New muxsess", sess.SessID(), this.muxsess == nil)
		this.muxsess = sess
		go this.servesess(sess)
	}
	log.Println("lsner done")
}

func (this *PeerConn) servesess(sess MuxSession) {
	for {
		stm, err := sess.AcceptStream()
		gopp.ErrPrint(err)
		if err != nil {
			break
		}

		go this.connectToBackend(stm)
	}
	log.Println("sess released")
}

func (this *PeerConn) writeout4tox(buf []byte) error {
	data := rudp.PfxBuffp().Get()
	data.Copy(buf)
	defer rudp.PfxBuffp().Put(data)
	// prior := false

	var err error
	fnum := this.peerno
	switch this.tptype {
	case TPTYPE_MESSAGE:
		_, err = this.tox.FriendSendMessage(fnum, string(data.FullData()))
	case TPTYPE_LOSSY:
		data.PPU8(254)
		err = this.tox.FriendSendLossyPacket(fnum, string(data.FullData()))
	case TPTYPE_LOSSLESS:
		data.PPU8(194)
		err = this.tox.FriendSendLosslessPacket(fnum, string(data.FullData()))
	}

	if err != nil {
		debug.Println(err)
	} else {
		debug.Println("kcp->tox:", data.FullLen(), time.Now().String())
	}

	return err
}

func (this *PeerConn) writeout4mtox(buf []byte) error {
	data := rudp.PfxBuffp().Get()
	data.Copy(buf)
	defer rudp.PfxBuffp().Put(data)
	// prior := false
	// return this.onKcpOutput2(data, nil, prior)
	return nil
}

// should block
func (this *PeerConn) connectToBackend(stm MuxStream) {
	// Dial
	var conn net.Conn
	var err error
	uo, err := url.Parse(stm.Syndat())
	gopp.ErrPrint(err, stm.Syndat())

	// TODO 如果连接失败，响应的包会导致client崩溃
	if uo.Scheme == "tcp" {
		conn, err = net.Dial("tcp", uo.Host)
	} else if uo.Scheme == "udp" {
		conn, err = net.Dial("udp", uo.Host)
	} else {
		log.Panicln("not supported proto:", uo.Scheme)
	}

	if err != nil {
		errl.Println(err, stm.StreamID(), stm.Syndat())
		// 连接结束
		debug.Println("dial failed, cleaning up...:", stm.StreamID(), stm.Syndat())
		// ch.server_socket_close = true
		// this.serverCloseChan <- ServerCloseEvent{ch}
		stm.Close()
		appevt.Trigger("connact", -1)
		return
	}
	info.Println("connected to:", conn.RemoteAddr().String(), stm.StreamID(), stm.Syndat())
	// info.Println("channel connected,", ch.chidcli, ch.chidsrv, ch.conv, pkt.msgid)

	var wg sync.WaitGroup
	wg.Add(2)

	// client close first
	clicloseC := make(chan bool, 0)
	rnsrv := int64(0)
	wnsrv := int64(0)
	// srv -> net
	go func() {
		wn, err := io.Copy(stm, conn)
		gopp.ErrPrint(err, wn)
		rnsrv = wn
		log.Println("xfer srv -> stm done", stm.StreamID(), wn)
		wg.Done()
		select {
		case <-clicloseC:
		case <-time.After(30 * time.Second):
			// stm.Close()
		}
	}()

	// net -> srv
	go func() {
		wn, err := io.Copy(conn, stm)
		gopp.ErrPrint(err, wn)
		wnsrv = wn
		log.Println("xfer stm -> srv done", stm.StreamID(), wn)
		wg.Done()
		conn.Close()
		clicloseC <- true
	}()

	wg.Wait()
	stm.Close()
	conn.Close()
	log.Println("release conn", stm.StreamID(), stm.Syndat(), "rnsrv", rnsrv, "wnsrv", wnsrv)
}

///
func (this *PeerConn) putnewconn(evt NewConnEvent) { this.newConnC <- evt }
func (this *PeerConn) servecli() {
	this.newConnC = make(chan NewConnEvent, mpcsz)
	// this.muxConnedC = make(chan bool, 1)
	// this.muxDisconnedC = make(chan bool, 1)

	for {
		// like event handler
		for {
			select {
			case evt := <-this.newConnC:
				this.muxConnHandler("newconn", evt)
				// case <-this.muxConnedC:
				//	this.muxConnHandler("muxconned", NewConnEvent{})
				// case <-this.muxDisconnedC:
				//	this.muxConnHandler("muxdisconned", NewConnEvent{})
			}
		}
	}
}

// should not block
func (this *PeerConn) muxConnHandler(evtname string, evt NewConnEvent) {
	switch evtname {
	case "newconn":
		muxsess := this.muxsess
		if muxsess == nil || (muxsess != nil && muxsess.IsClosed()) {
			log.Panicln("not possible")
			this.muxsess = nil
			if muxsess != nil {
				muxsess.Close()
				log.Println("mux1 conn is closed, reconnect ...", muxsess.SessID())
			}

			// this.newconnq = append(this.newconnq, evt)
			go this.connectmux1(evt.tname)
		} else {
			// server conn
			go this.serveconnevt(this.muxsess, evt)
		}

	case "muxconned":
		info.Println("muxconned")
		/*
			for _, evt_ := range this.newconnq {
				evt := evt_
				// server conn
				go this.serveconnevt(this.muxsess, evt)
			}
			this.newconnq = nil
		*/
	case "muxdisconned":
	default:
		log.Panicln("unknown evtname", evtname)
	}
}

func (this *PeerConn) connectmux1(tname string) error {
	for retry := 0; retry < 3; retry++ {
		err := this.connectmux1impl(tname, retry)
		if err == nil {
			break
		}
		time.Sleep(5 * time.Second)
	}
	return nil
}
func (this *PeerConn) connectmux1impl(tname string, retry int) error {
	info.Println("mux conning ...", retry)

	tunrec := config.getRecordByName(tname)

	addr := rudp.NewVtpAddr(fmt.Sprintf("toxrly.%s.%s", tunrec.tproto, tunrec.tname),
		fmt.Sprintf("%s:%v", tunrec.rhost, tunrec.rport))
	muxsess, err := RudpDialSync(this.mtvtp, addr)
	gopp.ErrPrint(err)
	if err != nil {
		return err
	}

	this.muxsess = muxsess
	// this.muxConnedC <- true
	return nil
}
func (this *PeerConn) serveconnevt(muxsess MuxSession, evt NewConnEvent) {
	// server conn
	conn := evt.conn
	times := evt.times
	btime := evt.btime
	tname := evt.tname
	this.serveconn(muxsess, conn, times, btime, tname)
}
func (this *PeerConn) serveconn(mux1 MuxSession, conn net.Conn, times int, btime time.Time, tname string) {
	info.Println("serving conn ...", tname)
	tunrec := config.getRecordByName(tname)
	btime2 := time.Now()
	syndat := fmt.Sprintf("%s://%s:%d/%s", tunrec.tproto, tunrec.rhost, tunrec.rport, tunrec.tname)

	var stm MuxStream
	var err error
	stmC := make(chan bool, 0)
	go func() {
		stm_, err_ := mux1.OpenStream(syndat)
		if err_ == nil {
			stm = stm_
			err = err_
			select {
			case stmC <- true:
			default:
				stm_.Close()
			}
		}
	}()
	select {
	case <-stmC:
	case <-time.After(10 * time.Second):
		err = fmt.Errorf("open steam timeout")
	}
	gopp.ErrPrint(err)
	if err != nil {
		if times >= 3 {
			log.Println("stm conn failed timeout", times, conn.RemoteAddr(), mux1.IsClosed(), err)
			mux1.Close()
			if this.muxsess != nil {
				this.muxsess.Close()
			}
			conn.Close()
			return
		}
		log.Println("stm conn failed, retry", times, conn.RemoteAddr(), mux1.IsClosed(), err)
		time.Sleep(3 * time.Second)
		this.newConnC <- NewConnEvent{conn, times + 1, btime, tname}
		return
	}
	log.Println("stm open time", stm.StreamID(), time.Since(btime2))

	var wg sync.WaitGroup
	wg.Add(2)

	// cli -> net
	go func() {
		wn, err := io.Copy(stm, conn)
		gopp.ErrPrint(err, stm.StreamID(), wn)
		info.Println("conn cli -> net done ", stm.StreamID())
		wg.Done()
		stm.Close()
		/*
			2019/01/23 21:13:05.354989 tunnelc.go:236: read tcp 127.0.0.1:8113->127.0.0.1:50844: use of closed network connection 8 13987221
			2019/01/23 21:13:05.355019 tunnelc.go:252: done cli conn 8 127.0.0.1:50844
		*/
	}()

	// net -> cli
	go func() {
		wn, err := io.Copy(conn, stm)
		gopp.ErrPrint(err, stm.StreamID(), wn)
		info.Println("conn net -> cli done ", stm.StreamID())
		wg.Done()

		select {
		case <-time.After(10 * time.Second):
			conn.Close()
		}
	}()

	wg.Wait()
	conn.Close()
	stm.Close()
	log.Println("done cli conn", stm.StreamID(), conn.RemoteAddr())
	return
}

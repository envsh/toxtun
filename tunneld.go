package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"net/url"
	"sync"
	"time"
	"unsafe"

	tox "github.com/TokTok/go-toxcore-c"
	"github.com/envsh/go-toxcore/mintox"
	"github.com/kitech/goplusplus"

	rudp "mkuse/rudp2"
)

type Tunneld struct {
	tox *tox.Tox
	// mtox *MTox

	// mux1     *muxone
	mtvtp    *rudp.Vtpconn
	muxlsner MuxListener

	toxPollChan  chan ToxPollEvent
	kcpInputChan chan ClientReadyReadEvent
}

func NewTunneld() *Tunneld {
	this := new(Tunneld)

	t := makeTox("toxtund")
	this.tox = t

	recs := config.recs
	t.SelfSetStatusMessage(fmt.Sprintf("%s of toxtun, %+v", "toxtuncs", recs))

	// callbacks
	t.CallbackSelfConnectionStatus(this.onToxnetSelfConnectionStatus, nil)
	t.CallbackFriendRequest(this.onToxnetFriendRequest, nil)
	t.CallbackFriendConnectionStatus(this.onToxnetFriendConnectionStatus, nil)
	t.CallbackFriendMessage(this.onToxnetFriendMessage, nil)
	t.CallbackFriendLossyPacket(this.onToxnetFriendLossyPacket, nil)
	t.CallbackFriendLosslessPacket(this.onToxnetFriendLosslessPacket, nil)

	// this.mtox = newMinTox("toxtund")
	// bcc, err := ioutil.ReadFile("./toxtunc.txt")
	// debug.Println(err)
	// bcc = bytes.TrimSpace(bcc)
	// pubkey := mintox.CBDerivePubkey(mintox.NewCryptoKeyFromHex(string(bcc)))
	// this.mtox.addFriend(pubkey.ToHex())
	// log.Println("cli pubkey?", pubkey.ToHex())

	///
	this.init()
	return this
}

func (this *Tunneld) init() {
	this.toxPollChan = make(chan ToxPollEvent, mpcsz)
	this.kcpInputChan = make(chan ClientReadyReadEvent, mpcsz)

	// callbacks
	// this.mtox.DataFunc = this.onMinToxData
	// this.mtox.startup()

	writeout := this.writeout4tox
	this.mtvtp = rudp.NewVtpconn(writeout)
	this.muxlsner = RudpListen(this.mtvtp)
	go this.serve()

	// install pollers
	go func() {
		tptime := time.Now()
		for {
			time.Sleep(time.Duration(smuse.tox_interval) * time.Millisecond)
			// time.Sleep(30 * time.Millisecond)
			// this.toxPollChan <- ToxPollEvent{}
			outit := false
			if time.Since(tptime) > 10*time.Second {
				tptime = time.Now()
				outit = true
			}
			if outit {
				log.Println("tox ittttttttttt ...")
			}
			iterate(this.tox)
			if outit {
				log.Println("tox ittttttttttt")
			}
		}
	}()

	for {
		select {
		case evt := <-this.kcpInputChan:
			this.processKcpInputChan(evt)
		case <-this.toxPollChan:
		}
	}
}

func (this *Tunneld) writeout4tox(buf []byte) error {
	data := rudp.PfxBuffp().Get()
	data.Copy(buf)
	defer rudp.PfxBuffp().Put(data)
	prior := false
	return this.onKcpOutput2(data, nil, prior)
}

func (this *Tunneld) writeout4mtox(buf []byte) error {
	data := rudp.PfxBuffp().Get()
	data.Copy(buf)
	defer rudp.PfxBuffp().Put(data)
	prior := false
	return this.onKcpOutput2(data, nil, prior)
}

func (this *Tunneld) serve() {
	// like event handler
	for {
		sess, err := this.muxlsner.Accept()
		gopp.ErrPrint(err)
		if err != nil {
			break
		}

		go this.servesess(sess)
	}
	log.Println("lsner done")
}

func (this *Tunneld) servesess(sess MuxSession) {
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

///////////

func (this *Tunneld) onKcpOutput2(buf *rudp.PfxByteArray, extra interface{}, prior bool) error {
	size := buf.FullLen()
	if size <= 0 {
		// 如果总是出现，并且不影响程序运行，那么也就不是bug了
		// info.Println("wtf")
		return fmt.Errorf("Invalid size %d", size)
	}
	debug.Println(size, "//", size, "//", string(gopp.SubBytes(buf.RawData(), 52)))
	// ch := extra.(*Channel)
	var toxtunid string
	for _, reco := range config.recs {
		toxtunid = reco.rpubkey
		break
	}
	toxtunid = "TODOTODO"
	fnum, err := this.tox.FriendByPublicKey(toxtunid)
	gopp.ErrPrint(err, toxtunid)
	if err != nil {
		return err
	}

	var sndlen int = size
	// var err error
	// err = this.mtox.sendData(buf, false, prior)
	buf.PPU8(254)
	err = this.tox.FriendSendLossyPacket(fnum, string(buf.FullData()))
	if err != nil {
		debug.Println(err)
	} else {
		debug.Println("kcp->tox:", sndlen, time.Now().String())
	}
	return err
}

func (this *Tunneld) onKcpOutput(buf *rudp.PfxByteArray, extra interface{}) {
	this.onKcpOutput2(buf, extra, false)
}

// should block
func (this *Tunneld) connectToBackend(stm MuxStream) {
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

var spdc2 = mintox.NewSpeedCalc()

func (this *Tunneld) onMinToxData(data []byte) {
	this.kcpInputChan <- ClientReadyReadEvent{nil, data, len(data), false}
}
func (this *Tunneld) processKcpInputChan(evt ClientReadyReadEvent) {
	err := this.mtvtp.Input(evt.buf)
	gopp.ErrPrint(err)
}

/*
func (this *Tunneld) handleDataPacket(buf []byte, friendNumber uint32) {
	// kcp包前4字段为conv，little hacky
	conv := binary.LittleEndian.Uint32(buf)
	if this.mux1 == nil {
		info.Println("mux1 conn not exist, drop pkt", conv)
		return
	}
	err := this.mux1.rudp_.Input(buf)
	gopp.ErrPrint(err)
	if err != nil {
		errl.Println("kcp input err:", conv, len(buf))
	} else {
		n := len(buf)
		debug.Println("tox->kcp:", conv, n, len(buf), gopp.StrSuf(string(buf), 52))
	}

}
*/
/*
func (this *Tunneld) handleCtrlPacket(pkt *Packet, friendId string) {
	if pkt.Command == CMDCONNSYN {
		info.Printf("New mux1 conn on tunnel %s to %s:%s:%s, conv: %d\n",
			pkt.Tunname, pkt.Tunproto, pkt.Remoteip, pkt.Remoteport, pkt.Conv)
		// close old, ack back, create new
		if this.mux1 != nil {
			mux1 := this.mux1
			this.mux1 = nil
			info.Println("mux1 conn exist, renew", mux1.conv, "->", pkt.Conv)
			mux1.Close()
		}

		repkt := &*pkt
		repkt.Command = CMDCONNACK
		pktdat := repkt.toJson()
		pba := rudp.NewPfxByteArray(len(pktdat))
		pba.Copy(pktdat)
		err := this.mtox.sendData(pba, true, true)
		if err != nil {
			errl.Println(err)
		}

		writeout := func(data *rudp.PfxByteArray, prior bool) error {
			return this.onKcpOutput2(data, nil, prior)
		}
		this.mux1 = NewMuxone(pkt.Conv, writeout)
		go this.acceptconn()

	} else if pkt.Command == CMDCLOSEFIN {
		log.Panicln("not used")
	} else {
		errl.Println("wtf, unknown cmmand:", pkt.Command, pkt.Chidcli, pkt.Chidsrv, pkt.Conv)
	}
}
func (this *Tunneld) acceptconn() {
	mux1 := this.mux1
	log.Println("mux1 accept loop start", mux1.conv, mux1.NumStreams())
	for {
		stm, err := mux1.AcceptStream()
		gopp.ErrPrint(err)
		if err != nil {
			break
		}
		log.Println("new stream conn", stm.ID(), stm.Syndat())
		// go this.connectToBackend(stm)
	}
	log.Println("mux1 accept loop done", mux1.conv, mux1.NumStreams())
	this.mux1 = nil
	mux1.Close()
}
*/

// a tool function
func (this *Tunneld) makeKcpConv(friendId string, pkt *Packet) uint32 {
	// crc32: toxid+host+port+time
	return makeKcpConv(friendId, pkt.Remoteip, pkt.Remoteport)
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
	livebotsOnFriendConnectionStatus(t, friendNumber, status)
	if status == 0 {
		appevt.Trigger("peeronline", false)
		appevt.Trigger("peeroffline")
	} else {
		appevt.Trigger("peeronline", true)
	}
}

func (this *Tunneld) onToxnetFriendMessage(t *tox.Tox, friendNumber uint32, message string, userData interface{}) {
	debug.Println(friendNumber, len(message), gopp.StrSuf(message, 52))
	friendId, err := this.tox.FriendGetPublicKey(friendNumber)
	if err != nil {
		errl.Println(err, friendId)
	}
}

func (this *Tunneld) onToxnetFriendLossyPacket(t *tox.Tox, friendNumber uint32, message string, userData interface{}) {
	debug.Println(friendNumber, len(message), gopp.StrSuf(message, 52), time.Now().String())
	buf := bytes.NewBufferString(message).Bytes()
	if buf[0] == 254 {
		data := buf[1:]
		this.kcpInputChan <- ClientReadyReadEvent{nil, data, len(data), false}
		debug.Println("tox->kcp:", len(buf), gopp.StrSuf(string(buf), 52))
	} else {
		info.Println("unknown message:", buf[0])
	}
}

func (this *Tunneld) onToxnetFriendLosslessPacket(t *tox.Tox, friendNumber uint32, message string, userData interface{}) {
	debug.Println(friendNumber, len(message), gopp.StrSuf(message, 52))
	buf := bytes.NewBufferString(message).Bytes()
	if buf[0] == 191 {
		buf = buf[1:]
		// kcp包前4字节为conv，little hacky
		if len(buf) < 4 {
			errl.Println("wtf")
		}
		conv := binary.LittleEndian.Uint32(buf)
		var ch unsafe.Pointer
		// ch := this.chpool.pool2[conv]
		if ch == nil {
			errl.Println("channel not found, maybe has some problem, maybe already closed", conv)
		} else {
			// n := ch.kcp.Input(buf, true, true)
			// debug.Println("tox->kcp:", conv, n, len(buf), gopp.StrSuf(string(buf), 52))
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

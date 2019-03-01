package main

import (
	"fmt"
	"gopp"
	"log"
	"math"
	"net"
	"sync"
	"time"

	tox "github.com/TokTok/go-toxcore-c"
)

type Tunnelx struct {
	tox *tox.Tox
	// mtox *MTox

	tptype     int
	dispatcher *Dispatcher
	pconns     sync.Map // friendNumber | public_key => *PeerConn

	// client new connect
	srvs     map[string]*TunListener
	newConnC chan NewConnEvent
}

func NewTunnelx() *Tunnelx {
	this := new(Tunnelx)
	this.tptype = TPTYPE_LOSSY

	// profname := "toxtunc"
	profname := config.srv_name
	t := makeTox(profname)
	this.tox = t

	recs := config.recs
	t.SelfSetStatusMessage(fmt.Sprintf("%s of toxtun, %+v", "toxtunx", recs))

	// callbacks
	t.CallbackSelfConnectionStatus(this.onToxnetSelfConnectionStatus, nil)
	t.CallbackFriendRequest(this.onToxnetFriendRequest, nil)
	t.CallbackFriendConnectionStatus(this.onToxnetFriendConnectionStatus, nil)

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

func (this *Tunnelx) init() {
	this.initsrv()
	this.initcli()

}

func (this *Tunnelx) serve() {
	// install pollers
	go toxiter(this.tox)

	select {}
}

// server
func (this *Tunnelx) initsrv() {
	tptype := this.tptype
	this.dispatcher = NewDispatcher(this.tox, tptype)

	for i := uint32(0); i < math.MaxUint32; i++ {
		if !this.tox.FriendExists(i) {
			break
		}
		this.makepeerconn(i)
	}
}

func (this *Tunnelx) makepeerconn(fnum uint32) {
	pconn := NewPeerConn(this.tox, fnum, this.tptype)
	this.pconns.Store(fnum, pconn)
	this.dispatcher.RegisterInputer(fnum, pconn.GetInputer())
}

// client
func (this *Tunnelx) initcli() {
	this.srvs = make(map[string]*TunListener)

	go this.servecli()
}

func (this *Tunnelx) servecli() {
	// recs := config.recs
	this.listenTunnels()

	this.newConnC = make(chan NewConnEvent, mpcsz)
	// this.muxConnedC = make(chan bool, 1)
	// this.muxDisconnedC = make(chan bool, 1)

	this.addrpubkeys()
	this.serveTunnels()

	// like event handler
	for {
		select {
		case evt := <-this.newConnC:
			this.muxConnHandler("newconn", evt)
			// case <-this.muxConnedC:
			// 	this.muxConnHandler("muxconned", NewConnEvent{})
			// case <-this.muxDisconnedC:
			//	this.muxConnHandler("muxdisconned", NewConnEvent{})
		}
	}
}

func (this *Tunnelx) addrpubkeys() {
	for tname, tunrec := range config.recs {
		_, err := this.tox.FriendByPublicKey(tunrec.rpubkey)
		if err != nil {
			fnum, err := this.tox.FriendAdd(tunrec.rpubkey, "hiyo")
			gopp.ErrPrint(err, tname, tunrec.rpubkey)
			if err == nil {
				this.makepeerconn(fnum)
			}
		}
	}
}
func (this *Tunnelx) listenTunnels() {
	for tname, tunrec := range config.recs {
		tunnelServerPort := tunrec.lport
		if tunrec.tproto == "tcp" {
			srv, err := net.Listen(tunrec.tproto, fmt.Sprintf(":%d", tunnelServerPort))
			if err != nil {
				log.Println(lerrorp, err)
				continue
			}
			this.srvs[tunrec.tname] = newTunListenerTcp(srv)
			// this.mtox.addFriend(tunrec.rpubkey)
		} else if tunrec.tproto == "udp" {
			srv, err := ListenUDP(fmt.Sprintf(":%d", tunnelServerPort))
			if err != nil {
				log.Println(lerrorp, err, tname, tunnelServerPort)
				continue
			}
			this.srvs[tunrec.tname] = newTunListenerUdp2(srv)
			// this.mtox.addFriend(tunrec.rpubkey)
		} else {
			log.Panicln("wtf,", tunrec)
		}
		log.Println(linfop, fmt.Sprintf("#T%s", tname), "tunaddr:", tunrec.tname, tunrec.tproto, tunrec.lhost, tunrec.lport)
	}
}

func (this *Tunnelx) serveTunnels() {
	srvs := this.srvs
	for tname, srv := range srvs {
		if srv.proto == "tcp" {
			go this.serveTunnelTcp(tname, srv.tcplsn)
		} else if srv.proto == "udp" {
			go this.serveTunnelUdp(tname, srv.fulsn)
			// go this.serveTunnelUdp(tname, srv.udplsn)
		}
	}
}

// should blcok
func (this *Tunnelx) serveTunnelTcp(tname string, srv net.Listener) {
	info.Println("serving tcp:", tname, srv.Addr())
	for {
		c, err := srv.Accept()
		if err != nil {
			info.Println(err)
		}
		// info.Println(c)
		info.Println("New conn :", c.RemoteAddr(), "->", c.LocalAddr(), tname)
		this.newConnC <- NewConnEvent{c, 0, time.Now(), tname}
		appevt.Trigger("newconn", tname)
	}
}

// should blcok
func (this *Tunnelx) serveTunnelUdp(tname string, srv net.Listener) {
	info.Println("serving udp:", tname, srv.Addr())
	for {
		c, err := srv.Accept()
		if err != nil {
			info.Println(err)
		}
		// info.Println(c)
		info.Println("New conn :", c.RemoteAddr(), "->", c.LocalAddr(), tname)
		this.newConnC <- NewConnEvent{c, 0, time.Now(), tname}
		appevt.Trigger("newconn", tname)
	}
}

func (this *Tunnelx) muxConnHandler(evtname string, evt NewConnEvent) {
	tunrec := config.getRecordByName(evt.tname)
	fnum, err := this.tox.FriendByPublicKey(tunrec.rpubkey)
	gopp.ErrPrint(err, evtname, evt.conn.LocalAddr())
	if err != nil {
		evt.conn.Close()
		return
	}

	pconnx, ok := this.pconns.Load(fnum)
	if !ok {
		log.Println("wtf pconn not exist", evtname, evt.conn.LocalAddr())
		evt.conn.Close()
		return
	}

	pconn := pconnx.(*PeerConn)
	pconn.putnewconn(evt)
}

////////////////
func (this *Tunnelx) onToxnetSelfConnectionStatus(t *tox.Tox, status int, extra interface{}) {
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

func (this *Tunnelx) onToxnetFriendRequest(t *tox.Tox, friendId string, message string, userData interface{}) {
	debug.Println(friendId, message)

	fnum, err := t.FriendAddNorequest(friendId)
	gopp.ErrPrint(err, friendId)
	if err == nil {
		t.WriteSavedata(tox_savedata_fname)
		this.makepeerconn(fnum)
	}
}

func (this *Tunnelx) onToxnetFriendConnectionStatus(t *tox.Tox, friendNumber uint32, status int, userData interface{}) {
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

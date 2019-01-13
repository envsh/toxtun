package main

import (
	"bytes"
	"fmt"
	"gopp"
	"io/ioutil"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/envsh/go-toxcore/mintox"
	// "github.com/pkg/errors"
	metrics "github.com/rcrowley/go-metrics"
	"github.com/sasha-s/go-deadlock"
	funk "github.com/thoas/go-funk"

	"mkuse/appcm"
)

type ClientInfo struct {
	tcpcli *mintox.TCPClient
	connid uint8
	status uint8 // always connid=16 here
	inuse  bool
	spdc   *mintox.SpeedCalc
	spditm *SpeedItem
}

type MTox struct {
	SelfPubkey *mintox.CryptoKey
	SelfSeckey *mintox.CryptoKey

	clismu    deadlock.RWMutex
	clis      map[string]*ClientInfo // binstr =>
	dhto      *mintox.DHT
	friendpks string
	friendpko *mintox.CryptoKey
	mtreg     metrics.Registry

	spdca    *mintox.SpeedCalc // total
	picker   *rrPick
	relays   []string // binstr of client pubkey
	relaysmu deadlock.RWMutex

	DataFunc   func(data []byte, cbdata mintox.Object, ctrl bool)
	DataCbdata mintox.Object
}

var tox_savedata_fname2 string

func newMinTox(name string) *MTox {
	tox_savedata_fname2 = fmt.Sprintf("./%s.txt", name)
	this := &MTox{}
	this.clis = make(map[string]*ClientInfo)
	this.spdca = mintox.NewSpeedCalc()
	this.picker = &rrPick{}
	// this.dhto = mintox.NewDHT()
	this.mtreg = metrics.NewRegistry()

	bcc, err := ioutil.ReadFile(tox_savedata_fname2)
	gopp.ErrFatal(err)
	bcc = bytes.TrimSpace(bcc)
	seckey := mintox.NewCryptoKeyFromHex(string(bcc))
	pubkey := mintox.CBDerivePubkey(seckey)
	if false {
		this.dhto.SetKeyPair(pubkey, seckey)
	}
	log.Println("My pubkey?", pubkey.ToHex())
	this.SelfPubkey, this.SelfSeckey = pubkey, seckey

	return this
}

func (this *MTox) startup() {
	this.setupTCPRelays()
	if tox_bs_group == "auto" {
		go this.daemonProc()
	}
}

func (this *MTox) setupTCPRelays() {
	tmpsrvs := append(append(append([]interface{}{}, cn_servers...), us_servers...), ru_servers...)
	// tmpsrvs = append(append([]interface{}{}, cn_servers...), us_servers...)
	tmpsrvs = servers
	for i := 0; i < len(tmpsrvs)/3; i++ {
		r := i * 3
		ipstr, port, pubkey := tmpsrvs[r+0].(string), tmpsrvs[r+1].(uint16), tmpsrvs[r+2].(string)
		clinfo := this.connectRelay(fmt.Sprintf("%s:%d", ipstr, port), pubkey)
		if tox_bs_group != "auto" {
		}
		clinfo.inuse = is_selected_server(pubkey)
		_ = clinfo
		clinfo.tcpcli.Startup()
	}
	if tox_bs_group != "auto" {
		tmpsrvs = servers
		for i := 0; i < len(tmpsrvs)/3; i++ {
			r := i * 3
			_, _, pubkey := tmpsrvs[r+0].(string), tmpsrvs[r+1].(uint16), tmpsrvs[r+2].(string)
			_ = pubkey
			// this.relays = append(this.relays, mintox.NewCryptoKeyFromHex(pubkey).BinStr())
			// this.addRelay(mintox.NewCryptoKeyFromHex(pubkey).BinStr())
		}
	}
}

func (this *MTox) connectRelay(ipaddr, pubkey string) *ClientInfo {
	log.Println("Connecting ", ipaddr, pubkey[:20])
	clinfo := &ClientInfo{}
	serv_pubkey := mintox.NewCryptoKeyFromHex(pubkey)
	clinfo.spdc = mintox.NewSpeedCalc()
	clinfo.spditm = newSpeedItem(serv_pubkey.BinStr(), ipaddr, this.mtreg)
	tcpcli := mintox.NewTCPClient(ipaddr, serv_pubkey, this.SelfPubkey, this.SelfSeckey)

	tcpcli.RoutingResponseFunc = this.onRoutingResponse
	tcpcli.RoutingResponseCbdata = tcpcli
	tcpcli.RoutingStatusFunc = this.onRoutingStatus
	tcpcli.RoutingStatusCbdata = tcpcli
	tcpcli.RoutingDataFunc = this.onRoutingData
	tcpcli.RoutingDataCbdata = tcpcli
	tcpcli.OnConfirmed = func() {
		log.Println(ldebugp, "adding friendpk", this.friendpks[:20], ipaddr)
		if this.friendpks != "" {
			err := tcpcli.ConnectPeer(this.friendpks)
			gopp.ErrPrint(err, lerrorp)
		}
	}
	tcpcli.OnClosed = this.onTCPClientClosed
	tcpcli.OnNetRecv = func(n int) { appcm.Meter("tcpnet.recv.len.total").Mark(int64(n)) }
	tcpcli.OnNetSent = func(n int) { appcm.Meter("tcpnet.sent.len.total").Mark(int64(n)) }
	tcpcli.OnReservedData = this.onReservedData

	clinfo.tcpcli = tcpcli
	this.clismu.Lock()
	defer this.clismu.Unlock()
	this.clis[serv_pubkey.BinStr()] = clinfo
	return clinfo
}

func (this *MTox) onTCPClientClosed(tcpcli *mintox.TCPClient) {
	ipaddr := tcpcli.ServAddr
	pubkey := tcpcli.ServPubkey.ToHex()
	pubkeyb := tcpcli.ServPubkey.BinStr()
	log.Println(lerrorp, "tcp client closed, cleanup...", ipaddr, pubkey[:20])
	this.removeRelay(pubkeyb)

	this.clismu.Lock()
	tcpcli.RoutingResponseFunc = nil
	tcpcli.RoutingResponseCbdata = nil
	tcpcli.RoutingStatusFunc = nil
	tcpcli.RoutingStatusCbdata = nil
	tcpcli.RoutingDataFunc = nil
	tcpcli.RoutingDataCbdata = nil
	tcpcli.OnConfirmed = nil
	tcpcli.OnClosed = nil
	tcpcli.OnNetRecv = nil
	tcpcli.OnNetSent = nil
	tcpcli.OnReservedData = nil

	if _, ok := this.clis[pubkeyb]; ok {
		delete(this.clis, pubkeyb)
	}
	this.clismu.Unlock()
	log.Println(lerrorp, "Reconnect after 5 seconds.", ipaddr, pubkey[:20])
	appcm.Counter(fmt.Sprintf("mintoxc.recontcpc.%s", strings.Split(ipaddr, ":")[0])).Inc(1)
	time.AfterFunc(5*time.Second, func() {
		clinfo := this.connectRelay(ipaddr, pubkey)
		clinfo.tcpcli.Startup()
	})
}

// pubkey: to connect friend's
func (this *MTox) onRoutingResponse(object mintox.Object, connid uint8, pubkey *mintox.CryptoKey) {
	tcpcli := object.(*mintox.TCPClient)
	log.Println(ldebugp, "routing new connid:", connid, tcpcli.ServAddr, pubkey.ToHex()[:20])
	this.clismu.Lock()
	defer this.clismu.Unlock()
	this.clis[tcpcli.ServPubkey.BinStr()].connid = connid
	this.clis[tcpcli.ServPubkey.BinStr()].status = 0 // 尚未建立连接
}

func (this *MTox) onRoutingStatus(object mintox.Object, number uint32, connid uint8, status uint8) {
	tcpcli := object.(*mintox.TCPClient)
	// if status > 0才是真正和建立了连接
	if true {
		log.Println(ldebugp, "routing status connid:", connid, status, tcpcli.ServAddr)
	}
	this.clismu.Lock()
	this.clis[tcpcli.ServPubkey.BinStr()].status = status
	this.clis[tcpcli.ServPubkey.BinStr()].connid = connid
	if status == 2 {
		this.sendRTTPing(tcpcli.ServPubkey.BinStr(), this.clis[tcpcli.ServPubkey.BinStr()])
	} else if status < 2 {
		// delete(this.conns[tcpcli.ServPubkey.BinStr()], connid)
	}
	this.clismu.Unlock()
	if status == 2 {
		this.addRelay(tcpcli.ServPubkey.BinStr())
	} else if status < 2 {
		// err := tcpcli.Close()
		// gopp.ErrPrint(err)
		if this.friendpks != "" {
			err := tcpcli.ConnectPeer(this.friendpks)
			gopp.ErrPrint(err)
		}
	}
}

func (this *MTox) onRoutingData(object mintox.Object, number uint32, connid uint8, data []byte, cbdata mintox.Object) {
	tcpcli := object.(*mintox.TCPClient)
	if false {
		log.Println(ldebugp, number, connid, len(data), tcpcli.ServAddr)
	}
	if bytes.HasPrefix(data, TCP_PACKET_TUNDATA) {
		this.DataFunc(data[len(TCP_PACKET_TUNDATA):], cbdata, false)
	} else if bytes.HasPrefix(data, TCP_PACKET_TUNCTRL) {
		this.DataFunc(data[len(TCP_PACKET_TUNCTRL):], cbdata, true)
	} else if bytes.HasPrefix(data, TCP_PACKET_BBTREQU) {
		this.sendBBTResp(tcpcli, data)
	} else if bytes.HasPrefix(data, TCP_PACKET_BBTRESP) {
		this.handleBBTResponse(tcpcli, data)
	} else if bytes.HasPrefix(data, TCP_PACKET_RTTPING) {
		this.sendRTTPong("", tcpcli, data)
	} else if bytes.HasPrefix(data, TCP_PACKET_RTTPONG) {
		this.clismu.Lock()
		spditm := this.clis[tcpcli.ServPubkey.BinStr()].spditm
		this.clismu.Unlock()
		// log.Println("rttpong:", time.Since(spditm.LastPingTime))
		spditm.RoundTripTime = (spditm.RoundTripTime + int(time.Since(spditm.LastPingTime).Seconds()*1000)) / 2
		spditm.RoundTripTime = spditm.RoundTripTime*2/10 + int(time.Since(spditm.LastPingTime).Seconds()*1000)*8/10
		spditm.mtRTT.Mark(int64(spditm.RoundTripTime))
	} else {
		log.Panicln("wtf", connid, len(data), tcpcli.ServAddr, string(data[:7]))
	}
	appcm.Meter("mintoxc.recv.cnt.total").Mark(1)
	appcm.Meter("mintoxc.recv.len.total").Mark(int64(len(data)))
	appcm.Meter(fmt.Sprintf("mintoxc.recv.len.%s", tcpcli.ServAddr)).Mark(int64(len(data)))

	this.clismu.Lock()
	clinfo := this.clis[tcpcli.ServPubkey.BinStr()]
	if clinfo.inuse {
		// meanspd := int(appcm.Meter("mintoxc.recv.len.total").Rate1())
		// clinfo.spditm.BottleneckBandwidth = gopp.IfElseInt(meanspd > clinfo.spditm.BottleneckBandwidth/2,
		//	meanspd, clinfo.spditm.BottleneckBandwidth)
		// clinfo.spditm.BottleneckBandwidth = meanspd
		if bytes.HasPrefix(data, TCP_PACKET_TUNDATA) || bytes.HasPrefix(data, TCP_PACKET_TUNCTRL) {
			// clinfo.spditm.LastUseTime = time.Now()
		}
	}
	this.clismu.Unlock()
}

func (this *MTox) addFriend(friendpk string) {
	log.Println(friendpk)
	pubkey := mintox.NewCryptoKeyFromHex(friendpk)
	// this.dhto.AddFriend(pubkey, nil, nil, 0)

	this.friendpks = friendpk
	this.friendpko = pubkey

	this.clismu.RLock()
	defer this.clismu.RUnlock()
	for _, clinfo := range this.clis {
		err := clinfo.tcpcli.ConnectPeer(friendpk)
		gopp.ErrPrint(err, lerrorp)
	}
}

var last_show_sent_speed time.Time

// 一种方式，上层的虚拟连接使用任意的一个连接发送数据
// 一种方式，上层的虚拟连接初始选择任意一个连接，并固定使用这一个。
func (this *MTox) sendData(data []byte, ctrl bool, prior bool) error {
	var err error
	var spdc *mintox.SpeedCalc
	var tcpcli *mintox.TCPClient
	data = append(gopp.IfElse(ctrl, TCP_PACKET_TUNCTRL, TCP_PACKET_TUNDATA).([]byte), data...)
	rlycnt := len(this.relays)
	if rlycnt == 0 {
		// this.calcPriority()
		rlycnt = len(this.relays)
	}
	rlycnt = gopp.IfElseInt(rlycnt > 2, 2, rlycnt)
	rlycnt = 1
	for tryi := 0; tryi < rlycnt; tryi++ {
		btime := time.Now()
		if ctrl {
			tcpcli, spdc, err = this.sendDataImpl(data, prior)
		} else {
			for i := 0; i < 1; i++ {
				tcpcli, spdc, err = this.sendDataImpl(data, prior)
			}
		}
		gopp.ErrPrint(err)
		dtime := time.Since(btime)
		if dtime > 1000*time.Millisecond {
			errl.Println(err, len(data), dtime)
		}
		if err == nil {
			appcm.Meter("mintoxc.sent.cnt.total").Mark(1)
			appcm.Meter("mintoxc.sent.len.total").Mark(int64(len(data)))
			this.spdca.Data(len(data))
			if false && int(time.Since(last_show_sent_speed).Seconds()) > 3 {
				last_show_sent_speed = time.Now()
				if spdc != nil {
					log.Printf("--- sent speed: %d/%d, len: %d/%d, %s\n",
						this.spdca.Avgspd, spdc.Avgspd, spdc.Totlen, len(data), tcpcli.ServAddr)
				} else {
					log.Printf("--- sent speed: %d, len: %d\n", this.spdca.Avgspd, len(data))
				}
			}
		}
		if err == nil {
			break
		}
		if err != nil && strings.Contains(err.Error(), "queue is full") {
			continue // retry next transport
		} else /*err != nil */ {
			break // don't care other error
		}
	}
	return err
}

var sendto = ""

func (this *MTox) sendDataImpl(data []byte, prior bool) (*mintox.TCPClient, *mintox.SpeedCalc, error) {
	var connid uint8

	itemid := this.selectRelay()
	if itemid == "" {
		err := fmt.Errorf("no peer connected relay candidate: %d", len(this.clis))
		log.Println(lwarningp, err)
		return nil, nil, err
	}

	var tcpcli *mintox.TCPClient
	var spdc *mintox.SpeedCalc
	this.clismu.RLock()
	clinfo, ok0 := this.clis[itemid]
	tcpcli = clinfo.tcpcli
	spdc = clinfo.spdc
	this.clismu.RUnlock()
	if !ok0 {
		log.Println(lwarningp, fmt.Errorf("cli not found"))
		return nil, nil, fmt.Errorf("cli not found")
	}
	connid = clinfo.connid
	if connid == 0 {
		err := fmt.Errorf("Invalid peer routing id: %d, servers: %d", connid, len(this.clis))
		log.Println(lwarningp, err)
		return nil, nil, err
	}
	if tcpcli == nil {
		log.Println(lwarningp, "not found tcpcli:", connid, len(this.clis))
		return nil, nil, fmt.Errorf("not found tcpcli: %d, %d", connid, len(this.clis))
	}

	if sendto != tcpcli.ServAddr {
		// log.Println("switch relay", sendto, tcpcli.ServAddr)
		// sendto = tcpcli.ServAddr
	}
	var err error
	_, err = tcpcli.SendDataPacket(connid, data, prior)
	gopp.ErrPrint(err)
	if err != nil {
		// return tcpcli, spdc, err
	}

	srvip := strings.Split(tcpcli.ServAddr, ":")[0]
	if err == nil {
		spdc.Data(len(data))
		appcm.Meter(fmt.Sprintf("mintoxc.sent.cnt.%s", srvip)).Mark(1)
		appcm.Meter(fmt.Sprintf("mintoxc.sent.len.%s", srvip)).Mark(int64(len(data)))
		// clinfo.spditm.LastUseTime = time.Now()
	}
	// sentcntm1 := appcm.Meter(fmt.Sprintf("mintoxc.sent.cnt.%s", srvip))
	sentlenm1 := appcm.Meter(fmt.Sprintf("mintoxc.sent.len.%s", srvip))
	sentcntname := fmt.Sprintf("mintoxc.sent.cnt.%s", srvip)
	timc.Mark(sentcntname) // include failed
	// rate: 1060 769.8741530645867 12.831235884409779 1307 15134 1060
	/*
		log.Printf("rate: %d, %.2f, %.2f, %d, %.2f, %d, %s\n",
			sentcntm1.Count(), sentcntm1.Rate1()*60, sentcntm1.Rate1(),
			len(data), sentlenm1.Rate1(), timc.Count1(sentcntname), srvip)
	*/
	if timc.Count1(sentcntname) > 300 && int(sentlenm1.Rate1()) > 500 { // 最近频繁使用，则认为速度可靠
		// log.Println("save real use speed as test speed:", clinfo.spditm.BottleneckBandwidth, "=>", int(sentlenm1.Rate1()))
		// clinfo.spditm.BottleneckBandwidth = int(sentlenm1.Rate1())
	}
	if err == nil {
		if int(sentlenm1.Rate1()) > 0 {
			// log.Println(clinfo.spditm.BottleneckBandwidth, int(sentlenm1.Rate1()), sentcntm1.Count(), srvip)
			// TODO BottleneckBandwidth r/w race
			// hmspdval := calchmval(float64(clinfo.spditm.BottleneckBandwidth), sentlenm1.Rate1(), float64(sentcntm1.Count()))
			// clinfo.spditm.BottleneckBandwidth = int(hmspdval)
		}
	}
	return tcpcli, spdc, err
}

// go's map is not very random, so use a rrPick to keep exact fair
func (this *MTox) selectRelay() string {
	if len(this.relays) == 0 {
		this.calcPriority()
	}
	if len(this.relays) < 3 {
		// log.Println("not enough candidate relays:", this.relays)
	}

	this.relaysmu.RLock()
	defer this.relaysmu.RUnlock()
	keys := this.relays
	itemid := this.picker.SelectOne(keys, func(item string) bool {
		this.clismu.RLock()
		defer this.clismu.RUnlock()

		if clinfo, ok := this.clis[item]; ok {
			if clinfo.status == 2 {
				return true
			}
		}
		return false
	})
	return itemid
}

func (this *MTox) addRelay(key string) {
	this.relaysmu.Lock()
	defer this.relaysmu.Unlock()

	if !funk.Contains(this.relays, key) {
		this.relays = append(this.relays, key)
	}
}
func (this *MTox) removeRelay(key string) {
	this.relaysmu.Lock()
	defer this.relaysmu.Unlock()

	newv := []string{}
	for _, k := range this.relays {
		if k == key {
		} else {
			newv = append(newv, k)
		}
	}
	this.relays = newv
}

type rrPick struct {
	next int
	mu   sync.RWMutex
}

func (this *rrPick) SelectOne(items []string, chkfn func(item string) bool) string {
	if len(items) == 0 {
		return ""
	}
	// this.next = 0
	oldlen := len(items)
	defer func() {
		errx := recover()
		if errx != nil {
			lines := strings.Split(fmt.Sprintf("%v", errx), "\n")
			log.Fatalln(lines[0], this.next, len(items), items == nil, oldlen)
		}
	}()

	// this.mu.Lock()
	// defer this.mu.Unlock()
	for i := 0; i < len(items); i++ {
		this.mu.Lock()
		if this.next >= len(items) {
			this.next = 0
		}
		item := items[this.next]
		this.next++
		this.mu.Unlock()

		if chkfn != nil {
			if chkfn(item) {
				return item
			}
		} else {
			return item
		}
	}

	return ""
}

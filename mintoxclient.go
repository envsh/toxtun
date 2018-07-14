package main

import (
	"bytes"
	"fmt"
	"gopp"
	"io/ioutil"
	"log"
	"strings"
	"time"

	"github.com/envsh/go-toxcore/mintox"
	"github.com/pkg/errors"
	"github.com/sasha-s/go-deadlock"

	"mkuse/appcm"
)

type MTox struct {
	SelfPubkey *mintox.CryptoKey
	SelfSeckey *mintox.CryptoKey

	clismu    deadlock.RWMutex
	clis      map[string]*mintox.TCPClient // binstr =>
	dhto      *mintox.DHT
	friendpks string
	friendpko *mintox.CryptoKey

	conns  map[string]map[uint8]uint8   // binstr => uint8 => , servpk => connid => status
	spdcs  map[string]*mintox.SpeedCalc // binstr => , send to relay server's speed
	spdca  *mintox.SpeedCalc            // total
	rlypks []string                     // binstr
	picker *rrPick

	DataFunc   func(data []byte, cbdata mintox.Object)
	DataCbdata mintox.Object
}

var tox_savedata_fname2 string

func newMinTox(name string) *MTox {
	tox_savedata_fname2 = fmt.Sprintf("./%s.txt", name)
	this := &MTox{}
	this.clis = make(map[string]*mintox.TCPClient)
	this.conns = make(map[string]map[uint8]uint8)
	this.spdcs = make(map[string]*mintox.SpeedCalc)
	this.spdca = mintox.NewSpeedCalc()
	this.picker = &rrPick{}
	// this.dhto = mintox.NewDHT()

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

	this.setupTCPRelays()
	return this
}

func (this *MTox) setupTCPRelays() {
	for i := 0; i < len(servers)/3; i++ {
		r := i * 3
		ipstr, port, pubkey := servers[r+0].(string), servers[r+1].(uint16), servers[r+2].(string)
		this.connectRelay(fmt.Sprintf("%s:%d", ipstr, port), pubkey)
	}
}

func (this *MTox) connectRelay(ipaddr, pubkey string) {
	log.Println("Connecting ", ipaddr, pubkey[:20])
	serv_pubkey := mintox.NewCryptoKeyFromHex(pubkey)
	this.clismu.Lock()
	this.conns[serv_pubkey.BinStr()] = make(map[uint8]uint8)
	this.spdcs[serv_pubkey.BinStr()] = mintox.NewSpeedCalc()
	this.clismu.Unlock()
	tcpcli := mintox.NewTCPClient(ipaddr, serv_pubkey, this.SelfPubkey, this.SelfSeckey)

	tcpcli.RoutingResponseFunc = this.onRoutingResponse
	tcpcli.RoutingResponseCbdata = tcpcli
	tcpcli.RoutingStatusFunc = this.onRoutingStatus
	tcpcli.RoutingStatusCbdata = tcpcli
	tcpcli.RoutingDataFunc = this.onRoutingData
	tcpcli.RoutingDataCbdata = tcpcli
	tcpcli.OnConfirmed = func() {
		if this.friendpks != "" {
			tcpcli.ConnectPeer(this.friendpks)
		}
	}
	tcpcli.OnClosed = this.onTCPClientClosed
	tcpcli.OnNetRecv = func(n int) { appcm.Meter("tcpnet.recv.len.total").Mark(int64(n)) }
	tcpcli.OnNetSent = func(n int) { appcm.Meter("tcpnet.sent.len.total").Mark(int64(n)) }

	this.clismu.Lock()
	this.clis[serv_pubkey.BinStr()] = tcpcli
	// this.conns[serv_pubkey.BinStr()] = make(map[uint8]uint8)
	this.rlypks = append(this.rlypks, serv_pubkey.BinStr())
	// this.spdcs[serv_pubkey.BinStr()] = mintox.NewSpeedCalc()
	this.clismu.Unlock()
}

func (this *MTox) onTCPClientClosed(tcpcli *mintox.TCPClient) {
	ipaddr := tcpcli.ServAddr
	pubkey := tcpcli.ServPubkey.ToHex()
	pubkeyb := tcpcli.ServPubkey.BinStr()
	log.Println("tcp client closed, cleanup...", ipaddr, pubkey[:20])
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

	if _, ok := this.clis[pubkeyb]; ok {
		delete(this.clis, pubkeyb)
	}
	if _, ok := this.conns[pubkeyb]; ok {
		delete(this.conns, pubkeyb)
	}
	if _, ok := this.spdcs[pubkeyb]; ok {
		delete(this.spdcs, pubkeyb)
	}
	rlypks := []string{}
	for _, rlypk := range this.rlypks {
		if rlypk != pubkeyb {
			rlypks = append(rlypks, rlypk)
		}
	}
	this.rlypks = rlypks
	this.clismu.Unlock()
	log.Println("Reconnect after 5 seconds.", ipaddr, pubkey[:20])
	appcm.Counter(fmt.Sprintf("mintoxc.recontcpc.%s", strings.Split(ipaddr, ":")[0])).Inc(1)
	time.AfterFunc(5*time.Second, func() { this.connectRelay(ipaddr, pubkey) })
}

// pubkey: to connect friend's
func (this *MTox) onRoutingResponse(object mintox.Object, connid uint8, pubkey *mintox.CryptoKey) {
	tcpcli := object.(*mintox.TCPClient)
	log.Println(ldebugp, connid, tcpcli.ServAddr, pubkey.ToHex()[:20])
	this.clismu.Lock()
	defer this.clismu.Unlock()
	this.conns[tcpcli.ServPubkey.BinStr()][connid] = 0
}

func (this *MTox) onRoutingStatus(object mintox.Object, number uint32, connid uint8, status uint8) {
	tcpcli := object.(*mintox.TCPClient)
	log.Println(ldebugp, connid, status, tcpcli.ServAddr)
	this.clismu.Lock()
	defer this.clismu.Unlock()
	this.conns[tcpcli.ServPubkey.BinStr()][connid] = status
	if status < 2 {
		// delete(this.conns[tcpcli.ServPubkey.BinStr()], connid)
	}
}

func (this *MTox) onRoutingData(object mintox.Object, number uint32, connid uint8, data []byte, cbdata mintox.Object) {
	tcpcli := object.(*mintox.TCPClient)
	log.Println(ldebugp, number, connid, len(data), tcpcli.ServAddr)
	this.DataFunc(data, cbdata)
	appcm.Meter("mintoxc.recv.cnt.total").Mark(1)
	appcm.Meter("mintoxc.recv.len.total").Mark(int64(len(data)))
	appcm.Meter(fmt.Sprintf("mintoxc.recv.len.%s", tcpcli.ServAddr)).Mark(int64(len(data)))
}

func (this *MTox) addFriend(friendpk string) {
	log.Println(friendpk)
	pubkey := mintox.NewCryptoKeyFromHex(friendpk)
	// this.dhto.AddFriend(pubkey, nil, nil, 0)

	this.friendpks = friendpk
	this.friendpko = pubkey

	this.clismu.RLock()
	defer this.clismu.RUnlock()
	for _, tcpcli := range this.clis {
		tcpcli.ConnectPeer(friendpk)
	}
}

// 一种方式，上层的虚拟连接使用任意的一个连接发送数据
// 一种方式，上层的虚拟连接初始选择任意一个连接，并固定使用这一个。
func (this *MTox) sendData(data []byte, ctrl bool) error {
	var err error
	var spdc *mintox.SpeedCalc
	var tcpcli *mintox.TCPClient
	btime := time.Now()
	if ctrl {
		tcpcli, spdc, err = this.sendDataImpl(data)
	} else {
		for i := 0; i < 1; i++ {
			tcpcli, spdc, err = this.sendDataImpl(data)
		}
	}
	dtime := time.Since(btime)
	if dtime > 1*time.Millisecond {
		errl.Println(err, len(data), dtime)
	}
	appcm.Meter("mintoxc.sent.cnt.total").Mark(1)
	appcm.Meter("mintoxc.sent.len.total").Mark(int64(len(data)))
	this.spdca.Data(len(data))
	if spdc != nil {
		log.Printf("--- sent speed: %d/%d, len: %d/%d, %s\n",
			this.spdca.Avgspd, spdc.Avgspd, spdc.Totlen, len(data), tcpcli.ServAddr)
	} else {
		log.Printf("--- sent speed: %d, len: %d\n", this.spdca.Avgspd, len(data))
	}
	return err
}
func (this *MTox) sendDataImpl(data []byte) (*mintox.TCPClient, *mintox.SpeedCalc, error) {
	var connid uint8
	var connbpk string

	itemid := this.selectRelay()
	this.clismu.RLock()
	if connsts, ok := this.conns[itemid]; ok {
		for connid_, status := range connsts {
			if status == 2 {
				connid = connid_
				connbpk = itemid
				break
			}
		}
	}
	this.clismu.RUnlock()
	if connid == 0 {
		log.Println(lwarningp, "no connection", len(this.conns))
		return nil, nil, errors.Errorf("no connection: %s", len(this.conns))
	}
	var tcpcli *mintox.TCPClient
	this.clismu.RLock()
	for s, tcpcli_ := range this.clis {
		if s == connbpk {
			tcpcli = tcpcli_
			break
		}
	}
	this.clismu.RUnlock()
	if tcpcli == nil {
		log.Println(lwarningp, "not found tcpcli:", connid, len(this.clis))
		return nil, nil, errors.Errorf("not found tcpcli: %d, %d", connid, len(this.clis))
	}
	_, err := tcpcli.SendDataPacket(connid, data)
	var spdc *mintox.SpeedCalc
	if spdc_, ok := this.spdcs[tcpcli.ServPubkey.BinStr()]; ok {
		spdc = spdc_
		spdc.Data(len(data))
		srvip := strings.Split(tcpcli.ServAddr, ":")[0]
		appcm.Meter(fmt.Sprintf("mintoxc.sent.cnt.%s", srvip)).Mark(1)
		appcm.Meter(fmt.Sprintf("mintoxc.sent.len.%s", srvip)).Mark(int64(len(data)))
	} else {
		log.Println("why not found spdc:", tcpcli.ServAddr, tcpcli.SelfPubkey.ToHex()[:20])
	}
	return tcpcli, spdc, err
}

// go's map is not very random, so use a rrPick to keep exact fair
func (this *MTox) selectRelay() string {
	this.clismu.RLock()
	itemid := this.picker.SelectOne(this.rlypks, func(item string) bool {
		if connsts, ok := this.conns[item]; ok {
			for _, st := range connsts {
				if st == 2 {
					return true
				}
			}
		}
		return false
	})
	this.clismu.RUnlock()
	return itemid
}

type rrPick struct {
	next int
}

func (this *rrPick) SelectOne(items []string, chkfn func(item string) bool) string {
	if len(items) == 0 {
		return ""
	}

	for i := 0; i < len(items); i++ {
		if this.next >= len(items) {
			this.next = 0
		}
		item := items[this.next]
		this.next++

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

package main

import (
	"bytes"
	"fmt"
	"gopp"
	"io/ioutil"
	"log"
	"time"

	"github.com/envsh/go-toxcore/mintox"
	"github.com/pkg/errors"
	"github.com/sasha-s/go-deadlock"
)

type MTox struct {
	SelfPubkey *mintox.CryptoKey
	SelfSeckey *mintox.CryptoKey

	clismu    deadlock.RWMutex
	clis      map[string]*mintox.TCPClient // binstr =>
	dhto      *mintox.DHT
	friendpks string
	friendpko *mintox.CryptoKey

	conns map[string]map[uint8]uint8 // binstr => uint8 => , servpk => connid => status

	DataFunc   func(data []byte, cbdata mintox.Object)
	DataCbdata mintox.Object
}

var tox_savedata_fname2 string

func newMinTox(name string) *MTox {
	tox_savedata_fname2 = fmt.Sprintf("./%s.txt", name)
	this := &MTox{}
	this.clis = make(map[string]*mintox.TCPClient)
	this.conns = make(map[string]map[uint8]uint8)
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
	serv_pubkey := mintox.NewCryptoKeyFromHex(pubkey)
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

	this.clis[serv_pubkey.BinStr()] = tcpcli
	this.conns[serv_pubkey.BinStr()] = make(map[uint8]uint8)
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
var spdc = mintox.NewSpeedCalc()

func (this *MTox) sendData(data []byte, ctrl bool) error {
	var err error
	btime := time.Now()
	if ctrl {
		err = this.sendDataImpl(data)
	} else {
		for i := 0; i < 1; i++ {
			err = this.sendDataImpl(data)
		}
	}
	dtime := time.Since(btime)
	if dtime > 1*time.Millisecond {
		errl.Println(err, len(data), dtime)
	}
	spdc.Data(len(data))
	log.Printf("--- sent speed: %d, len: %d\n", spdc.Avgspd, len(data))
	return err
}
func (this *MTox) sendDataImpl(data []byte) error {
	var connid uint8
	var connbpk string
	var tmpnum int

	this.clismu.RLock()
	for bpk, connsts := range this.conns {
		for connid_, status := range connsts {
			if status == 2 {
				connid = connid_
				connbpk = bpk
				break
			}
		}
		if connid > 0 {
			break
		}
		tmpnum += 1
	}
	this.clismu.RUnlock()
	if connid == 0 {
		log.Println(lwarningp, "no connection", len(this.conns))
		return errors.Errorf("no connection: %s", len(this.conns))
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
		return errors.Errorf("not found tcpcli: %d, %d", connid, len(this.clis))
	}
	_, err := tcpcli.SendDataPacket(connid, data)
	return err
}

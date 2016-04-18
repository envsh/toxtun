package main

import (
	"net"
	"strings"
	"sync"
	"time"

	"github.com/bitly/go-simplejson"
)

const (
	CMDKEYCHANIDCLIENT = "chidcli"
	CMDKEYCHANIDSERVER = "chidsrv"
	CMDKEYCOMMAND      = "cmd"
	CMDKEYDATA         = "data"
	CMDKEYREMIP        = "remip"
	CMDKEYREMPORT      = "remport"
	CMDKEYCONV         = "conv"
	CMDKEYMSGID        = "msgid"

	CMDCONNSYN  = "connect_syn"
	CMDCONNACK  = "connect_ack"
	CMDCLOSEFIN = "close_fin"
	// CMDCLOSEACK = "close_ack"
	CMDSENDDATA = "send_data"
)

var chanid0 = 10000
var chanidlock sync.Mutex
var msgid0 = uint64(10000)
var msgidlock sync.Mutex

func nextChanid() int {
	chanidlock.Lock()
	defer chanidlock.Unlock()

	id := chanid0
	chanid0 += 1
	return id
}

func nextMsgid() uint64 {
	msgidlock.Lock()
	defer msgidlock.Unlock()

	id := msgid0
	msgid0 += 1
	return id
}

type Channel struct {
	state   int
	conn    net.Conn
	chidcli int
	chidsrv int
	ip      string
	port    string
	conv    uint32
	toxid   string // 仅用于服务器端
	kcp     *KCP

	// 关闭状态
	client_socket_close bool
	client_kcp_close    bool
	server_socket_close bool
	server_kcp_close    bool

	// 连接超时，客户端使用
	conn_begin_time time.Time
	conn_try_times  int
	conn_ack_recved bool

	close_reason []string
}

func NewChannelClient(conn net.Conn) *Channel {
	ch := new(Channel)
	ch.chidcli = nextChanid()
	ch.conn = conn
	ch.conn_begin_time = time.Now()
	ch.close_reason = make([]string, 0)

	return ch
}

func NewChannelWithId(chanid int) *Channel {
	ch := new(Channel)
	ch.chidsrv = nextChanid()
	ch.chidcli = chanid
	return ch
}

func NewChannelFromPacket(pkt *Packet) *Channel {
	ch := new(Channel)
	ch.chidcli = pkt.chidcli
	ch.chidsrv = pkt.chidsrv
	ch.conv = pkt.conv

	return ch
}

func (this *Channel) makeConnectSYNPacket() *Packet {
	pkt := NewPacket(this, CMDCONNSYN, "")
	pkt.conv = this.conv
	return pkt
}

func (this *Channel) makeConnectACKPacket() *Packet {
	pkt := NewPacket(this, CMDCONNACK, "")
	pkt.conv = this.conv
	return pkt
}

func (this *Channel) makeDataPacket(data string) *Packet {
	pkt := NewPacket(this, CMDSENDDATA, data)
	pkt.conv = this.conv
	return pkt
}

func (this *Channel) makeCloseFINPacket() *Packet {
	pkt := NewPacket(this, CMDCLOSEFIN, "")
	pkt.conv = this.conv
	return pkt
}

/*
func (this *Channel) makeCloseACKPacket() *Packet {
	pkt := NewPacket(this, CMDCLOSEACK, "")
	pkt.conv = this.conv
	return pkt
}
*/

func (this *Channel) addCloseReason(reason string) {
	for i := 0; i < len(this.close_reason); i++ {
		if reason == this.close_reason[i] {
			info.Println("reason already exists, maybe loop,", reason, this.closeReason())
			break
		}
	}

	this.close_reason = append(this.close_reason, reason)
	if len(this.close_reason) > 5 {
		info.Println(this.chidcli, this.chidsrv, this.conv)
		panic("wtf")
	}
}

func (this *Channel) closeReason() string {
	return strings.Join(this.close_reason, ",")
}

///////////
type ChannelPool struct {
	pool  map[int]*Channel
	pool2 map[uint32]*Channel
}

func NewChannelPool() *ChannelPool {
	p := new(ChannelPool)
	p.pool = make(map[int]*Channel, 0)
	p.pool2 = make(map[uint32]*Channel, 0)

	return p
}
func (this *ChannelPool) putClient(ch *Channel) {
	this.pool[ch.chidcli] = ch
	if ch.conv > 0 {
		this.pool2[ch.conv] = ch
	}
	appevt.Trigger("chanact", 1, len(this.pool), len(this.pool2))
}

// put lacked
func (this *ChannelPool) putClientLacks(ch *Channel) {
	if _, ok := this.pool[ch.chidcli]; !ok {
	}
	this.pool2[ch.conv] = ch
	appevt.Trigger("chanact", 0, len(this.pool), len(this.pool2))
}

func (this *ChannelPool) putServer(ch *Channel) {
	this.pool[ch.chidsrv] = ch
	this.pool2[ch.conv] = ch
	appevt.Trigger("chanact", 1, len(this.pool), len(this.pool2))
}

func (this *ChannelPool) rmClient(ch *Channel) {
	if _, ok := this.pool[ch.chidcli]; !ok {
		panic(ch.chidcli)
	}
	delete(this.pool, ch.chidcli)
	if _, ok := this.pool2[ch.conv]; !ok {
		// panic(ch.conv)
	}
	delete(this.pool2, ch.conv)
	appevt.Trigger("chanact", -1, len(this.pool), len(this.pool2))
}

func (this *ChannelPool) rmServer(ch *Channel) {
	delete(this.pool, ch.chidsrv)
	delete(this.pool2, ch.conv)
	appevt.Trigger("chanact", -1, len(this.pool), len(this.pool2))
}

////////////

type Packet struct {
	chidcli    int
	chidsrv    int
	command    string
	data       string
	remoteip   string
	remoteport string
	conv       uint32
	msgid      uint64
}

func NewPacket(ch *Channel, command string, data string) *Packet {
	return &Packet{chidcli: ch.chidcli, chidsrv: ch.chidsrv, command: command, data: data,
		remoteip: ch.ip, remoteport: ch.port, msgid: nextMsgid()}
}

func NewBrokenPacket(conv uint32) *Packet {
	pkt := new(Packet)
	pkt.conv = conv
	pkt.msgid = nextMsgid()
	return pkt
}

func (this *Packet) isconnsyn() bool {
	return this.command == CMDCONNSYN
}

func (this *Packet) isconnack() bool {
	return this.command == CMDCONNACK
}

func (this *Packet) isdata() bool {
	return this.command == CMDSENDDATA
}

func (this *Packet) toJson() []byte {
	jso := simplejson.New()
	jso.Set(CMDKEYCHANIDCLIENT, this.chidcli)
	jso.Set(CMDKEYCHANIDSERVER, this.chidsrv)
	jso.Set(CMDKEYCOMMAND, this.command)
	jso.Set(CMDKEYDATA, this.data)
	jso.Set(CMDKEYREMIP, this.remoteip)
	jso.Set(CMDKEYREMPORT, this.remoteport)
	jso.Set(CMDKEYCONV, this.conv)
	jso.Set(CMDKEYMSGID, this.msgid)

	jsb, err := jso.Encode()
	if err != nil {
		return nil
	}
	return jsb
}

func parsePacket(buf []byte) *Packet {
	jso, err := simplejson.NewJson(buf)
	if err != nil {
		return nil
	}

	pkt := new(Packet)
	pkt.chidcli = jso.Get(CMDKEYCHANIDCLIENT).MustInt()
	pkt.chidsrv = jso.Get(CMDKEYCHANIDSERVER).MustInt()
	pkt.command = jso.Get(CMDKEYCOMMAND).MustString()
	pkt.data = jso.Get(CMDKEYDATA).MustString()
	pkt.remoteip = jso.Get(CMDKEYREMIP).MustString()
	pkt.remoteport = jso.Get(CMDKEYREMPORT).MustString()
	pkt.conv = uint32(jso.Get(CMDKEYCONV).MustUint64())
	pkt.msgid = jso.Get(CMDKEYMSGID).MustUint64()

	return pkt
}

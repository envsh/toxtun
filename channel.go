package main

import (
	"net"
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

	CMDCONNACK   = "connect_ack"
	CMDCONNFIN   = "connect_fin"
	CMDSENDDATA  = "send_data"
	CMDCLOSEACK  = "close_ack"
	CMDCLOSEFIN1 = "close_fin1"
	CMDCLOSEFIN2 = "close_fin2"
)

var chanid = 10000
var chanlock sync.Mutex

func nextChanid() int {
	chanlock.Lock()
	defer chanlock.Unlock()

	id := chanid
	chanid += 1
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
}

func NewChannelClient(conn net.Conn) *Channel {
	ch := new(Channel)
	ch.chidcli = nextChanid()
	ch.conn = conn
	ch.conn_begin_time = time.Now()

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

func (this *Channel) makeConnectACKPacket() *Packet {
	pkt := NewPacket(this, CMDCONNACK, "")
	pkt.conv = this.conv
	return pkt
}

func (this *Channel) makeConnectFINPacket() *Packet {
	pkt := NewPacket(this, CMDCONNFIN, "")
	pkt.conv = this.conv
	return pkt
}

func (this *Channel) makeDataPacket(data string) *Packet {
	pkt := NewPacket(this, CMDSENDDATA, data)
	pkt.conv = this.conv
	return pkt
}

func (this *Channel) makeCloseACKPacket() *Packet {
	pkt := NewPacket(this, CMDCLOSEACK, "")
	pkt.conv = this.conv
	return pkt
}

func (this *Channel) makeCloseFIN1Packet() *Packet {
	pkt := NewPacket(this, CMDCLOSEFIN1, "")
	pkt.conv = this.conv
	return pkt
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
	this.pool2[ch.conv] = ch
}

func (this *ChannelPool) putServer(ch *Channel) {
	this.pool[ch.chidsrv] = ch
	this.pool2[ch.conv] = ch
}

func (this *ChannelPool) rmClient(ch *Channel) {
	delete(this.pool, ch.chidcli)
	delete(this.pool2, ch.conv)
}

func (this *ChannelPool) rmServer(ch *Channel) {
	delete(this.pool, ch.chidsrv)
	delete(this.pool2, ch.conv)
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
}

func NewPacket(ch *Channel, command string, data string) *Packet {
	return &Packet{chidcli: ch.chidcli, chidsrv: ch.chidsrv, command: command, data: data,
		remoteip: ch.ip, remoteport: ch.port}
}

func (this *Packet) isconnack() bool {
	return this.command == CMDCONNACK
}

func (this *Packet) isconnfin() bool {
	return this.command == CMDCONNFIN
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

	return pkt
}

package main

import (
	"net"
	"sync"

	"github.com/bitly/go-simplejson"
)

const (
	CMDKEYCHANIDCLIENT = "chidcli"
	CMDKEYCHANIDSERVER = "chidsrv"
	CMDKEYCOMMAND      = "cmd"
	CMDKEYDATA         = "data"
	CMDKEYREMIP        = "remip"
	CMDKEYREMPORT      = "remport"

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
	chidcli int
	chidsrv int
	state   int
	conn    net.Conn
	ip      string
	port    string
}

func NewChannelClient(conn net.Conn) *Channel {
	ch := new(Channel)
	ch.chidcli = nextChanid()
	ch.conn = conn

	return ch
}

func NewChannelWithId(chanid int) *Channel {
	ch := new(Channel)
	ch.chidsrv = nextChanid()
	ch.chidcli = chanid
	return ch
}

func (this *Channel) makeConnectACKPacket() *Packet {
	return NewPacket(this, CMDCONNACK, "")
}

func (this *Channel) makeConnectFINPacket() *Packet {
	return NewPacket(this, CMDCONNFIN, "")
}

func (this *Channel) makeDataPacket(data string) *Packet {
	return NewPacket(this, CMDSENDDATA, data)
}

///////////
type ChannelPool struct {
	pool map[int]*Channel
}

func NewChannelPool() *ChannelPool {
	p := new(ChannelPool)
	p.pool = make(map[int]*Channel, 0)

	return p
}
func (this *ChannelPool) putClient(ch *Channel) {
	this.pool[ch.chidcli] = ch
}

func (this *ChannelPool) putServer(ch *Channel) {
	this.pool[ch.chidsrv] = ch
}

////////////

type Packet struct {
	chidcli    int
	chidsrv    int
	command    string
	data       string
	remoteip   string
	remoteport string
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

	return pkt
}

package main

import (
	"encoding/base64"
	"net"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/bitly/go-simplejson"
	"github.com/vmihailenco/msgpack"
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

var chanid0 int = 10000
var chanidlock sync.Mutex
var msgid0 uint64 = 10000
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

	close_reasons []string
	close_stacks  [][]uintptr
	rmctimes      int
}

func NewChannelClient(conn net.Conn) *Channel {
	ch := new(Channel)
	ch.chidcli = nextChanid()
	ch.conn = conn
	ch.conn_begin_time = time.Now()
	ch.close_reasons = make([]string, 0)
	ch.close_stacks = make([][]uintptr, 0)

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
	ch.chidcli = pkt.Chidcli
	ch.chidsrv = pkt.Chidsrv
	ch.conv = pkt.Conv

	return ch
}

func (this *Channel) makeConnectSYNPacket() *Packet {
	pkt := NewPacket(this, CMDCONNSYN, "")
	pkt.Conv = this.conv
	return pkt
}

func (this *Channel) makeConnectACKPacket() *Packet {
	pkt := NewPacket(this, CMDCONNACK, "")
	pkt.Conv = this.conv
	return pkt
}

func (this *Channel) makeDataPacket(data string) *Packet {
	pkt := NewPacket(this, CMDSENDDATA, data)
	pkt.Conv = this.conv
	return pkt
}

func (this *Channel) makeCloseFINPacket() *Packet {
	pkt := NewPacket(this, CMDCLOSEFIN, "")
	pkt.Conv = this.conv
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
	for i := 0; i < len(this.close_reasons); i++ {
		if reason == this.close_reasons[i] {
			info.Println("reason already exists, maybe loop,", reason, this.closeReason())
			break
		}
	}

	this.close_reasons = append(this.close_reasons, reason)
	if len(this.close_reasons) > 5 {
		info.Println(this.chidcli, this.chidsrv, this.conv)
		panic("wtf")
	}
}

func (this *Channel) closeReason() string {
	return strings.Join(this.close_reasons, ",")
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

func dumpStacks(pcs []uintptr) {
	for idx, pc := range pcs {
		fn := runtime.FuncForPC(pc)
		file, line := fn.FileLine(pc)
		info.Println(idx, fn.Name(), file, line)
	}
}

func (this *ChannelPool) rmClient(ch *Channel) {
	ch.rmctimes += 1
	haserr := false

	if _, ok := this.pool[ch.chidcli]; !ok {
		errl.Println("maybe already removed.", ch.chidsrv, ch.chidsrv, ch.conv)
		dumpStacks(ch.close_stacks[len(ch.close_stacks)-1])
		info.Println("=======")
		pcs := make([]uintptr, 16)
		pcn := runtime.Callers(1, pcs)
		dumpStacks(pcs[0:pcn])
		// panic(ch.chidcli)
	} else {
		pcs := make([]uintptr, 16)
		pcn := runtime.Callers(1, pcs)
		ch.close_stacks = append(ch.close_stacks, pcs[0:pcn])
		delete(this.pool, ch.chidcli)
	}

	if _, ok := this.pool2[ch.conv]; !ok {
		// panic(ch.conv)
	} else {
		delete(this.pool2, ch.conv)
	}

	if haserr {
		if ch.rmctimes > 2 {
			// go func() {
			errl.Println("errinfo:", ch.rmctimes, len(this.pool), len(this.pool2))
			panic(ch.chidcli)
			// }()
		} else {
			errl.Println("errinfo:", ch.rmctimes, len(this.pool), len(this.pool2))
		}
	}
	appevt.Trigger("chanact", -1, len(this.pool), len(this.pool2))
}

func (this *ChannelPool) rmServer(ch *Channel) {
	delete(this.pool, ch.chidsrv)
	delete(this.pool2, ch.conv)
	appevt.Trigger("chanact", -1, len(this.pool), len(this.pool2))
}

////////////
const pkt_use_fmt = "msgp" // "json"
type Packet struct {
	Chidcli    int
	Chidsrv    int
	Command    string
	Data       string
	Remoteip   string
	Remoteport string
	Conv       uint32
	Msgid      uint64
}

func NewPacket(ch *Channel, command string, data string) *Packet {
	return &Packet{Chidcli: ch.chidcli, Chidsrv: ch.chidsrv, Command: command, Data: data,
		Remoteip: ch.ip, Remoteport: ch.port, Msgid: nextMsgid()}
}

func NewBrokenPacket(conv uint32) *Packet {
	pkt := new(Packet)
	pkt.Conv = conv
	pkt.Msgid = nextMsgid()
	return pkt
}

func (this *Packet) isconnsyn() bool {
	return this.Command == CMDCONNSYN
}

func (this *Packet) isconnack() bool {
	return this.Command == CMDCONNACK
}

func (this *Packet) isdata() bool {
	return this.Command == CMDSENDDATA
}

func (this *Packet) toJson() []byte {
	if pkt_use_fmt == "msgp" {
		bcc, err := base64.StdEncoding.DecodeString(this.Data)
		if err != nil {
			info.Println(err)
			return nil
		} else {
			nthis := *this
			nthis.Data = string(bcc)
			return nthis.toMsgPackImpl()
		}
	}
	return this.toJsonImpl()
}

func (this *Packet) toJsonImpl() []byte {
	jso := simplejson.New()
	jso.Set(CMDKEYCHANIDCLIENT, this.Chidcli)
	jso.Set(CMDKEYCHANIDSERVER, this.Chidsrv)
	jso.Set(CMDKEYCOMMAND, this.Command)
	jso.Set(CMDKEYDATA, this.Data)
	jso.Set(CMDKEYREMIP, this.Remoteip)
	jso.Set(CMDKEYREMPORT, this.Remoteport)
	jso.Set(CMDKEYCONV, this.Conv)
	jso.Set(CMDKEYMSGID, this.Msgid)

	jsb, err := jso.Encode()
	if err != nil {
		return nil
	}
	return jsb
}

func parsePacket(buf []byte) *Packet {
	if pkt_use_fmt == "msgp" {
		pkt := parsePacketFromMsgPack(buf)
		if pkt == nil {
			return nil
		}
		npkt := *pkt
		npkt.Data = base64.StdEncoding.EncodeToString([]byte(pkt.Data))
		return &npkt
	}
	return parsePacketFromJson(buf)
}

func parsePacketFromJson(buf []byte) *Packet {
	jso, err := simplejson.NewJson(buf)
	if err != nil {
		return nil
	}

	pkt := new(Packet)
	pkt.Chidcli = jso.Get(CMDKEYCHANIDCLIENT).MustInt()
	pkt.Chidsrv = jso.Get(CMDKEYCHANIDSERVER).MustInt()
	pkt.Command = jso.Get(CMDKEYCOMMAND).MustString()
	pkt.Data = jso.Get(CMDKEYDATA).MustString()
	pkt.Remoteip = jso.Get(CMDKEYREMIP).MustString()
	pkt.Remoteport = jso.Get(CMDKEYREMPORT).MustString()
	pkt.Conv = uint32(jso.Get(CMDKEYCONV).MustUint64())
	pkt.Msgid = jso.Get(CMDKEYMSGID).MustUint64()

	return pkt
}

// msgpack
func (this *Packet) toMsgPackImpl() []byte {
	pkt, err := msgpack.Marshal(this)
	if err != nil {
		return nil
	}
	return pkt
}

func parsePacketFromMsgPack(buf []byte) *Packet {
	pkto := &Packet{}
	err := msgpack.Unmarshal(buf, pkto)
	if err != nil {
		return nil
	}
	return pkto
}

package main

import (
	"encoding/base64"
	"flag"
	"net"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/bitly/go-simplejson"
	proto "github.com/golang/protobuf/proto"
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

var chanid0 int32 = 10000
var chanidlock sync.Mutex
var msgid0 uint64 = 10000
var msgidlock sync.Mutex

func nextChanid() int32 {
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
	chidcli int32
	chidsrv int32
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

func NewChannelWithId(chanid int32) *Channel {
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
	pkt := NewPacket(this, CMDCONNSYN, []byte(""))
	pkt.Conv = this.conv
	return pkt
}

func (this *Channel) makeConnectACKPacket() *Packet {
	pkt := NewPacket(this, CMDCONNACK, []byte(""))
	pkt.Conv = this.conv
	return pkt
}

func (this *Channel) makeDataPacket(data []byte) *Packet {
	pkt := NewPacket(this, CMDSENDDATA, data)
	pkt.Conv = this.conv
	return pkt
}

func (this *Channel) makeCloseFINPacket() *Packet {
	pkt := NewPacket(this, CMDCLOSEFIN, []byte(""))
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
	pool  map[int32]*Channel
	pool2 map[uint32]*Channel
}

func NewChannelPool() *ChannelPool {
	p := new(ChannelPool)
	p.pool = make(map[int32]*Channel, 0)
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
var pkt_use_fmt = "json" // "msgpack" // "json" // "protobuf"
func init() {
	flag.StringVar(&pkt_use_fmt, "pktfmt", pkt_use_fmt, "packet format: json/msgpack/protobuf")
}

type Packet struct {
	Version    int32  `protobuf:"varint,1,opt,name=Version" json:"ver,omitempty" msgpack:"ver,omitempty"`
	Chidcli    int32  `protobuf:"varint,2,opt,name=Chidcli" json:"ccid,omitempty" msgpack:"ccid,omitempty"`
	Chidsrv    int32  `protobuf:"varint,3,opt,name=Chidsrv" json:"scid,omitempty" msgpack:"scid,omitempty"`
	Command    string `protobuf:"bytes,4,opt,name=Command" json:"cmd,omitempty" msgpack:"cmd,omitempty"`
	Remoteip   string `protobuf:"bytes,5,opt,name=Remoteip" json:"rip,omitempty" msgpack:"rip,omitempty"`
	Remoteport string `protobuf:"bytes,6,opt,name=Remoteport" json:"rpt,omitempty" msgpack:"rpt,omitempty"`
	Conv       uint32 `protobuf:"varint,7,opt,name=Conv" json:"conv,omitempty" msgpack:"conv,omitempty"`
	Msgid      uint64 `protobuf:"varint,8,opt,name=Msgid" json:"mid,omitempty" msgpack:"mid,omitempty"`
	Compress   bool   `protobuf:"varint,9,opt,name=Compress" json:"cpr,omitempty" msgpack:"cpr,omitempty"`
	Data       []byte `protobuf:"bytes,10,opt,name=Data" json:"dat,omitempty" msgpack:"dat,omitempty"`
}

// proto.Message interface methods
func (m *Packet) Reset()         { *m = Packet{} }
func (m *Packet) String() string { return proto.CompactTextString(m) }
func (*Packet) ProtoMessage()    {}

func NewPacket(ch *Channel, command string, data []byte) *Packet {
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
	if this.isdata() { // not need this info anymore
		this.Remoteip = ""
		this.Remoteport = ""
	}
	if pkt_use_fmt == "msgpack" {
		return this.toMsgPackImpl()
	} else if pkt_use_fmt == "protobuf" {
		return this.toProtobufImpl()
	}
	return this.toJsonImpl()
}

func (this *Packet) toJsonImpl() []byte {
	jso := simplejson.New()
	jso.Set(CMDKEYCHANIDCLIENT, this.Chidcli)
	jso.Set(CMDKEYCHANIDSERVER, this.Chidsrv)
	jso.Set(CMDKEYCOMMAND, this.Command)
	jso.Set(CMDKEYDATA, this.Data)
	if !this.isdata() {
		jso.Set(CMDKEYREMIP, this.Remoteip)
		jso.Set(CMDKEYREMPORT, this.Remoteport)
	}
	jso.Set(CMDKEYCONV, this.Conv)
	jso.Set(CMDKEYMSGID, this.Msgid)

	jsb, err := jso.Encode()
	if err != nil {
		info.Println(err)
		return nil
	}
	return jsb
}

func parsePacket(buf []byte) *Packet {
	if pkt_use_fmt == "msgpack" {
		pkt := parsePacketFromMsgPack(buf)
		if pkt == nil {
			return nil
		}
		return pkt
	} else if pkt_use_fmt == "protobuf" {
		pkt := parsePacketFromProtobuf(buf)
		if pkt == nil {
			return nil
		}
		return pkt
	}
	return parsePacketFromJson(buf)
}

func parsePacketFromJson(buf []byte) *Packet {
	jso, err := simplejson.NewJson(buf)
	if err != nil {
		info.Println(err)
		return nil
	}

	pkt := new(Packet)
	pkt.Chidcli = int32(jso.Get(CMDKEYCHANIDCLIENT).MustInt())
	pkt.Chidsrv = int32(jso.Get(CMDKEYCHANIDSERVER).MustInt())
	pkt.Command = jso.Get(CMDKEYCOMMAND).MustString()
	if !pkt.isdata() {
		pkt.Remoteip = jso.Get(CMDKEYREMIP).MustString()
		pkt.Remoteport = jso.Get(CMDKEYREMPORT).MustString()
	}
	pkt.Conv = uint32(jso.Get(CMDKEYCONV).MustUint64())
	pkt.Msgid = jso.Get(CMDKEYMSGID).MustUint64()
	// pkt.Data = jso.Get(CMDKEYDATA).MustString()
	pkt.Data, _ = base64.StdEncoding.DecodeString(jso.Get(CMDKEYDATA).MustString())

	return pkt
}

// msgpack
func (this *Packet) toMsgPackImpl() []byte {
	pkt, err := msgpack.Marshal(this)
	if err != nil {
		info.Println(err)
		return nil
	}
	return pkt
}

func parsePacketFromMsgPack(buf []byte) *Packet {
	pkto := &Packet{}
	err := msgpack.Unmarshal(buf, pkto)
	if err != nil {
		info.Println(err)
		return nil
	}
	return pkto
}

// protobuf
func (this *Packet) toProtobufImpl() []byte {
	pkt, err := proto.Marshal(this)
	if err != nil {
		info.Println(err)
		return nil
	}
	return pkt
}

func parsePacketFromProtobuf(buf []byte) *Packet {
	pkto := &Packet{}
	err := proto.Unmarshal(buf, pkto)
	if err != nil {
		info.Println(err)
		return nil
	}
	return pkto
}

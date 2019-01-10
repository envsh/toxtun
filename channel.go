package main

import (
	"encoding/base64"
	"flag"
	"net"
	"sync"

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
	CMDKEYTNAME        = "tunam"
	CMDKEYTPROTO       = "tproto"

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
	tname   string
	tproto  string
	state   int
	conn    net.Conn
	chidcli int32
	chidsrv int32
	ip      string
	port    string
	conv    uint32
	toxid   string // 仅用于服务器端

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
	Tunname    string `protobuf:"varint,9,opt,name=Tunname" json:"tnm,omitempty" msgpack:"tnm,omitempty"`
	Tunproto   string `protobuf:"varint,10,opt,name=Tunproto" json:"tpt,omitempty" msgpack:"tpt,omitempty"`
	Compress   bool   `protobuf:"varint,11,opt,name=Compress" json:"cpr,omitempty" msgpack:"cpr,omitempty"`
	Data       []byte `protobuf:"bytes,12,opt,name=Data" json:"dat,omitempty" msgpack:"dat,omitempty"`
}

// proto.Message interface methods
func (m *Packet) Reset()         { *m = Packet{} }
func (m *Packet) String() string { return proto.CompactTextString(m) }
func (*Packet) ProtoMessage()    {}

func NewPacket(ch *Channel, command string, data []byte) *Packet {
	return &Packet{Chidcli: ch.chidcli, Chidsrv: ch.chidsrv, Command: command, Data: data,
		Remoteip: ch.ip, Remoteport: ch.port, Msgid: nextMsgid(), Tunname: ch.tname}
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
	jso.Set(CMDKEYTNAME, this.Tunname)
	jso.Set(CMDKEYCHANIDCLIENT, this.Chidcli)
	jso.Set(CMDKEYCHANIDSERVER, this.Chidsrv)
	jso.Set(CMDKEYCOMMAND, this.Command)
	jso.Set(CMDKEYDATA, this.Data)
	if !this.isdata() {
		jso.Set(CMDKEYREMIP, this.Remoteip)
		jso.Set(CMDKEYREMPORT, this.Remoteport)
		jso.Set(CMDKEYTPROTO, this.Tunproto)
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
	pkt.Tunname = jso.Get(CMDKEYTNAME).MustString()
	pkt.Chidcli = int32(jso.Get(CMDKEYCHANIDCLIENT).MustInt())
	pkt.Chidsrv = int32(jso.Get(CMDKEYCHANIDSERVER).MustInt())
	pkt.Command = jso.Get(CMDKEYCOMMAND).MustString()
	if !pkt.isdata() {
		pkt.Remoteip = jso.Get(CMDKEYREMIP).MustString()
		pkt.Remoteport = jso.Get(CMDKEYREMPORT).MustString()
		pkt.Tunproto = jso.Get(CMDKEYTPROTO).MustString()
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

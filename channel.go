package main

import (
	"encoding/base64"
	"errors"
	"flag"
	"net"
	"runtime"
	"strconv"
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
	CMDKEYTNAME        = "tunam"
	CMDKEYTPROTO       = "tproto"
	CMDKEYSRCHOST      = "srch"

	CMDCONNSYN  = "connect_syn"
	CMDCONNACK  = "connect_ack"
	CMDCLOSEFIN = "close_fin"
	// CMDCLOSEACK = "close_ack"
	CMDSENDDATA = "send_data"
)

// chanid < 100000 use as UDP channel id
var chanid0 int32 = 100000
var chanidlock sync.Mutex
var msgid0 uint64 = 100000
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

type UnionConn struct {
	tcpc net.Conn
	udpc net.PacketConn
}

func (this *UnionConn) Close() error {
	if this.tcpc != nil {
		return this.tcpc.Close()
	}
	if this.udpc != nil {
		return this.udpc.Close()
	}
	return nil
}

func (this *UnionConn) Read(b []byte) (int, error) {
	if this.tcpc != nil {
		return this.tcpc.Read(b)
	}
	return 0, errors.New("conn nil")
}

func (this *UnionConn) Write(b []byte) (int, error) {
	if this.tcpc != nil {
		return this.tcpc.Write(b)
	}
	return 0, errors.New("conn nil")
}

func (this *UnionConn) ReadFrom(b []byte) (int, net.Addr, error) {
	if this.udpc != nil {
		return this.udpc.ReadFrom(b)
	}
	return 0, nil, errors.New("conn nil")
}

func (this *UnionConn) WriteTo(b []byte, addr net.Addr) (int, error) {
	if this.udpc != nil {
		return this.udpc.WriteTo(b, addr)
	}
	return 0, errors.New("conn nil")
}

func (this *UnionConn) WriteToHost(b []byte, addr string) (int, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return 0, err
	}
	ipo := net.ParseIP(host)
	porti, _ := strconv.Atoi(port)
	addro := &net.UDPAddr{IP: ipo, Port: porti}
	return this.WriteTo(b, addro)
}

func (this *UnionConn) IsNil() bool { return this.udpc == nil && this.tcpc == nil }

//
type Channel struct {
	tname  string
	tproto string
	state  int
	// conn    net.Conn
	conn    *UnionConn
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

func NewChannelClientFromUnion(conn *UnionConn, tname string) *Channel {
	ch := new(Channel)
	ch.tname = tname
	ch.chidcli = nextChanid()
	if conn.udpc != nil || conn.tcpc != nil {
		ch.conn = conn
	}
	ch.conn_begin_time = time.Now()
	ch.close_reasons = make([]string, 0)
	ch.close_stacks = make([][]uintptr, 0)

	if config != nil {
		tunrec := config.getRecordByName(tname)
		ch.tproto = tunrec.tproto
	}

	return ch
}

func NewChannelClientFromUdp(conn net.PacketConn, tname string) *Channel {
	return NewChannelClientFromUnion(&UnionConn{udpc: conn}, tname)
}

// default FromTcp
func NewChannelClient(conn net.Conn, tname string) *Channel {
	return NewChannelClientFromUnion(&UnionConn{tcpc: conn}, tname)
}

func NewChannelWithId(chanid int32, tname string) *Channel {
	ch := new(Channel)
	ch.tname = tname
	ch.chidsrv = nextChanid()
	ch.chidcli = chanid

	if config != nil {
		tunrec := config.getRecordByName(tname)
		ch.tproto = tunrec.tproto
	}

	return ch
}

func NewChannelFromPacket(pkt *Packet) *Channel {
	ch := new(Channel)
	ch.tname = pkt.Tunname
	ch.tproto = pkt.Tunproto
	ch.chidcli = pkt.Chidcli
	ch.chidsrv = pkt.Chidsrv
	ch.conv = pkt.Conv

	return ch
}

func (this *Channel) makeConnectSYNPacket() *Packet {
	pkt := NewPacket(this, CMDCONNSYN, []byte(""))
	pkt.Conv = this.conv
	pkt.Tunname = this.tname
	pkt.Tunproto = this.tproto
	return pkt
}

func (this *Channel) makeConnectACKPacket() *Packet {
	pkt := NewPacket(this, CMDCONNACK, []byte(""))
	pkt.Conv = this.conv
	pkt.Tunname = this.tname
	pkt.Tunproto = this.tproto
	return pkt
}

func (this *Channel) makeDataPacket(data []byte) *Packet {
	pkt := NewPacket(this, CMDSENDDATA, data)
	pkt.Conv = this.conv
	pkt.Tunname = this.tname
	pkt.Tunproto = this.tproto
	return pkt
}

func (this *Channel) makeCloseFINPacket() *Packet {
	pkt := NewPacket(this, CMDCLOSEFIN, []byte(""))
	pkt.Conv = this.conv
	pkt.Tunname = this.tname
	pkt.Tunproto = this.tproto
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

func (this *Channel) dumpInfo() []interface{} {
	ch := this
	return []interface{}{ch.chidcli, ch.chidsrv, ch.conv, ch.tname, ch.tproto}
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
// lack what?
func (this *ChannelPool) putClientLacks(ch *Channel) {
	if _, ok := this.pool[ch.chidcli]; !ok {
	}
	this.pool2[ch.conv] = ch
	appevt.Trigger("chanact", 0, len(this.pool), len(this.pool2))
}

func (this *ChannelPool) putServer(ch *Channel) {
	if ch.chidsrv != 0 {
		this.pool[ch.chidsrv] = ch
	}
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
	Tunname    string `protobuf:"varint,9,opt,name=Tunname" json:"tnm,omitempty" msgpack:"tnm,omitempty"`
	Tunproto   string `protobuf:"varint,10,opt,name=Tunproto" json:"tpt,omitempty" msgpack:"tpt,omitempty"`
	Srchost    string `protobuf:"varint,10,opt,name=Srchost" json:"srch,omitempty" msgpack:"srch,omitempty"`
	Compress   bool   `protobuf:"varint,11,opt,name=Compress" json:"cpr,omitempty" msgpack:"cpr,omitempty"`
	Data       []byte `protobuf:"bytes,12,opt,name=Data" json:"dat,omitempty" msgpack:"dat,omitempty"`
}

// proto.Message interface methods
func (m *Packet) Reset()         { *m = Packet{} }
func (m *Packet) String() string { return proto.CompactTextString(m) }
func (*Packet) ProtoMessage()    {}

func NewPacket(ch *Channel, command string, data []byte) *Packet {
	return &Packet{Chidcli: ch.chidcli, Chidsrv: ch.chidsrv, Command: command, Data: data,
		Remoteip: ch.ip, Remoteport: ch.port, Msgid: nextMsgid(),
		Tunname: ch.tname, Tunproto: ch.tproto}
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
	if this.isdata() && this.Tunproto == "tcp" { // not need this info anymore
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
	jso.Set(CMDKEYTPROTO, this.Tunproto)
	jso.Set(CMDKEYCHANIDCLIENT, this.Chidcli)
	jso.Set(CMDKEYCHANIDSERVER, this.Chidsrv)
	jso.Set(CMDKEYCOMMAND, this.Command)
	jso.Set(CMDKEYDATA, this.Data)
	if !this.isdata() || this.Tunproto == "udp" {
		jso.Set(CMDKEYREMIP, this.Remoteip)
		jso.Set(CMDKEYREMPORT, this.Remoteport)
		jso.Set(CMDKEYSRCHOST, this.Srchost)
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
	pkt.Tunproto = jso.Get(CMDKEYTPROTO).MustString()
	pkt.Chidcli = int32(jso.Get(CMDKEYCHANIDCLIENT).MustInt())
	pkt.Chidsrv = int32(jso.Get(CMDKEYCHANIDSERVER).MustInt())
	pkt.Command = jso.Get(CMDKEYCOMMAND).MustString()
	if !pkt.isdata() || pkt.Tunproto == "udp" {
		pkt.Remoteip = jso.Get(CMDKEYREMIP).MustString()
		pkt.Remoteport = jso.Get(CMDKEYREMPORT).MustString()
		pkt.Srchost = jso.Get(CMDKEYSRCHOST).MustString()
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

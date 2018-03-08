package main

import (
	"fmt"
	"log"
	"net"
	"reflect"
	"sync"
)

/*
插件式抽象传输层，tox/udp/ethernum/...
不负责丢包处理功能。(kcp transport 除外)
*/
type TransportBase struct {
	name_                 string
	enabled               bool
	isServer              bool // 是否是server模式
	weight                int  // 传输包时的权重
	lossy                 bool
	readyReadNoticeChan   chan CommonEvent // 无数据
	readyReadDataChanType reflect.Type
	localVirtAddr_        string
	// peerVirtAddr_         string // in Channel
}

func (this *TransportBase) name() string          { return this.name_ }
func (this *TransportBase) localVirtAddr() string { return this.localVirtAddr_ }
func (this *TransportBase) sendBufferFull() bool  { return false }

/*
就目前实现的transport，它们之间的角色关系：
DirectUdpTransport
ToxLossyTransport
ToxLosslessTransport
EthereumTransport
GroupTransport
KcpTransport

KcpTransport和ToxLosslessTransport是类似TCP的可靠传输。
其他是类似UDP的不可靠传输。
GroupTransport是类似MPUDP的多路径不可靠传输。
除了KcpTransport之外，其他的传输在不可靠传输角度是可以相互替换的。
*/

// featureTypes
const (
	FT_Lossless = 0
)

type Transport interface {
	init() bool
	serve()
	// TODO don't export like this api
	getReadyReadChan() <-chan CommonEvent
	getReadyReadChanType() reflect.Type
	getEventData(evt CommonEvent) ([]byte, int, interface{})
	// blocked
	// readData() ([]byte, int, interface{})
	sendData(data string, to string) error
	localVirtAddr() string
	name() string
	sendBufferFull() bool
	shutdown()
}

// TODO only port mode
// TODO singleton udpSrv for one instance
type DirectUdpTransport struct {
	TransportBase
	udpSrv net.PacketConn
	// readyReadDataChan chan UdpReadyReadEvent
	peerIP  string
	localIP string
	port    int
	// peerAddr net.Addr

	shutWG sync.WaitGroup
}

func NewDirectUdpTransport() *DirectUdpTransport {
	tp := newDirectUdpTransport()
	obip := getOutboundIp()
	if !isReservedIpStr(obip) {
		tp.enabled = true
	} else {
		log.Println(lwarningp, "reseved ip detected, disable DirectUdpTransport:", obip)
	}

	tp.isServer = true
	tp.localIP = obip

	tp.init()
	go tp.serve()
	return tp
}

func NewDirectUdpTransportClient(srvip string) *DirectUdpTransport {
	tp := newDirectUdpTransport()
	if srvip != "" {
		obip := srvip
		if !isReservedIpStr(obip) {
			tp.enabled = true
		} else {
			log.Println(lwarningp, "reseved ip detected, disable DirectUdpTransport:", obip)
		}
	} else {
		// TODO
		log.Println(lwarningp, "not supply srvip")
	}

	tp.isServer = false
	tp.peerIP = srvip
	tp.localIP = getOutboundIp()

	tp.init()
	go tp.serve()
	return tp
}

func newDirectUdpTransport() *DirectUdpTransport {
	tp := &DirectUdpTransport{}
	tp.name_ = "udp"
	tp.lossy = true
	return tp
}

func (this *DirectUdpTransport) init() bool {
	this.readyReadNoticeChan = make(chan CommonEvent, mpcsz)
	// this.readyReadDataChan = make(chan UdpReadyReadEvent, mpcsz)
	this.readyReadDataChanType = reflect.TypeOf(this.readyReadNoticeChan)

	if this.isServer {
		return this.initServer()
	} else {
		return this.initServer()
	}
}

/////
func (this *DirectUdpTransport) initServer() bool {
	log.Println(ldebugp)
	for i := 0; i < 256; i++ {
		addr := fmt.Sprintf(":%d", 18588+i)
		udpSrv, err := net.ListenPacket("udp", addr)
		if err != nil {
			// log.Println(lerrorp, err, udpSrv)
		} else {
			log.Println(linfop, "Listen UDP:", udpSrv.LocalAddr().String())
			this.udpSrv = udpSrv
			this.port = 18588 + i
			this.localVirtAddr_ = fmt.Sprintf("%s:%d", this.localIP, this.port)
			break
		}
	}
	if this.udpSrv == nil {
		log.Fatalf("can not listen UDP port: (%d, %d)\n", 18588, 18588+256)
	}
	return true
}

func (this *DirectUdpTransport) initClient() bool {
	// connect to server?
	return true
}

func (this *DirectUdpTransport) serve() {
	log.Println(ldebugp, this.localIP, this.peerIP)
	if this.isServer {
		this.serveServer()
	} else {
		this.serveServer()
	}
}

func (this *DirectUdpTransport) serveServer() {
	if this.udpSrv == nil {
		log.Fatalln("not listen")
	}

	this.shutWG.Add(1)
	stop := false
	for !stop {
		// transport 读取到的时编码后的长度, tunmtu*2
		// 一般KCP.Input == -2
		buf := make([]byte, 2345)
		rdn, addr, err := this.udpSrv.ReadFrom(buf)
		if err != nil {
			log.Println(lerrorp, rdn, addr, err)
			break
		} else {
			// TODO 发送太快的问题
			// this.peerAddr = addr
			evt := UdpReadyReadEvent{addr, buf[0:rdn], rdn}
			this.readyReadNoticeChan <- newCommonEvent(evt)
			log.Println(ldebugp, "net->udp:", rdn, addr.String())
		}
	}
	this.shutWG.Done()
}
func (this *DirectUdpTransport) serveClient() {
}

func (this *DirectUdpTransport) shutdown() {
	log.Println(ldebugp, "closing udp transport...")
	this.udpSrv.Close()
	this.shutWG.Wait()
	log.Println(ldebugp, "closed udp transport.")
}

func (this *DirectUdpTransport) getEventData(evt CommonEvent) ([]byte, int, interface{}) {
	devt := evt.v.Interface().(UdpReadyReadEvent)
	return devt.buf, devt.size, devt.addr
}

func (this *DirectUdpTransport) getReadyReadChanType() reflect.Type {
	return this.readyReadDataChanType
}

func (this *DirectUdpTransport) getReadyReadChan() <-chan CommonEvent {
	return this.readyReadNoticeChan
}

func (this *DirectUdpTransport) getConn() interface{} {
	return nil
}

func (this *DirectUdpTransport) sendDataBytes(buf []byte, size int, uaddr string) int {
	if this.isServer {
		return this.sendDataServer(buf, size, uaddr)
	} else {
		return this.sendDataClient(buf, size, uaddr)
	}
}
func (this *DirectUdpTransport) sendData(buf string, toaddr string) error {
	if this.isServer {
		this.sendDataServer([]byte(buf), len(buf), toaddr)
	} else {
		this.sendDataClient([]byte(buf), len(buf), toaddr)
	}
	return nil
}

func (this *DirectUdpTransport) sendDataServer(buf []byte, size int, uaddr string) int {
	// unused(uaddr) // we don't use passed uaddr
	/*
		if this.peerAddr == nil {
			log.Println(lwarningp, "still not got the peerAddr")
			return -1
		}
	*/
	peerAddr, err := net.ResolveUDPAddr("udp", uaddr)
	if err != nil {
		log.Println(lerrorp, err, uaddr, peerAddr)
		return -1
	}

	// wrn, err := this.udpSrv.WriteTo(buf[:size], this.peerAddr)
	wrn, err := this.udpSrv.WriteTo(buf[:size], peerAddr)
	if err != nil {
		log.Println(lerrorp, err, wrn)
	} else {
		log.Println(ldebugp, "udp->net:", wrn)
	}
	return wrn
}

func (this *DirectUdpTransport) sendDataClient(buf []byte, size int, toaddr string) int {
	uaddr, err := net.ResolveUDPAddr("udp", toaddr)
	if err != nil {
		log.Println(lerrorp, err, uaddr, toaddr)
		return -1
	}

	// replace with? this.sendDataServer(buf, size, uaddr)
	wrn, err := this.udpSrv.WriteTo(buf[:size], uaddr)
	if err != nil {
		log.Println(lerrorp, err, wrn)
	}
	return wrn
}

type ToxMessageTransport struct {
	TransportBase
}

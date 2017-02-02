package main

import (
	"fmt"
	"log"
	"net"
	"reflect"
)

/*
插件式抽象传输层，tox/udp/ethernum/...
不负责丢包处理功能。(kcp transport 除外)
*/
type TransportBase struct {
	name                  string
	enabled               bool
	isServer              bool // 是否是server模式
	weight                int  // 传输包时的权重
	lossy                 bool
	readyReadNoticeChan   chan CommonEvent // 无数据
	readyReadDataChanType reflect.Type
	localVirtAddr_        string
	// peerVirtAddr_         string // in Channel
}

type Transport interface {
	init() bool
	serve()
	getReadyReadChan() <-chan CommonEvent
	getReadyReadChanType() reflect.Type
	getEventData(evt CommonEvent) ([]byte, int, interface{})
	sendData(data string, to string) error
	localVirtAddr() string
}

// TODO only port mode
type DirectUdpTransport struct {
	TransportBase
	udpSrv net.PacketConn
	// readyReadDataChan chan UdpReadyReadEvent
	peerIP  string
	localIP string
	port    int
	// peerAddr net.Addr
}

func NewDirectUdpTransport() *DirectUdpTransport {
	tp := newDirectUdpTransport()
	obip := getOutboundIp()
	if !isReservedIpStr(obip) {
		tp.enabled = true
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
	tp.name = "udp"
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

func (this *DirectUdpTransport) localVirtAddr() string {
	return this.localVirtAddr_
}

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
		log.Fatalln("can not listen UDP port: (%d, %d)", 18588, 18588+256)
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

	stop := false
	for !stop {
		buf := make([]byte, 1600)
		rdn, addr, err := this.udpSrv.ReadFrom(buf)
		if err != nil {
			log.Println(lerrorp, rdn, addr, err)
		} else {
			// this.peerAddr = addr
			evt := UdpReadyReadEvent{addr, buf[0:rdn], rdn}
			this.readyReadNoticeChan <- newCommonEvent(evt)
			log.Println(ldebugp, "net->udp:", rdn, addr.String())
		}
	}
}

func (this *DirectUdpTransport) getEventData(evt CommonEvent) ([]byte, int, interface{}) {
	devt := evt.v.Interface().(UdpReadyReadEvent)
	return devt.buf, devt.size, devt.addr
}

func (this *DirectUdpTransport) serveClient() {

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

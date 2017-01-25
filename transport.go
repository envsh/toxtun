package main

import (
	"log"
	"net"
	"reflect"
)

/*
插件式抽象传输层，tox/udp/ethernum/...
*/
type TransportBase struct {
	enable                bool
	server                bool // 是否是server模式
	weight                int  // 传输包时的权重
	lossy                 bool
	readyReadNoticeChan   chan CommonEvent // 无数据
	readyReadDataChanType reflect.Type
}

type Transport interface {
	init() bool
	serve()
	getReadyReadChan() <-chan CommonEvent
	getReadyReadChanType() reflect.Type
	getEventData(evt CommonEvent) ([]byte, int, interface{})
	sendData(data string) error
}

type DirectUdpTransport struct {
	TransportBase
	udpSrv            net.PacketConn
	readyReadDataChan chan UdpReadyReadEvent
	peerIP            string
	localIP           string
}

func NewDiectUdpTransport() *DirectUdpTransport {
	tp := &DirectUdpTransport{}
	obip := getOutboundIp()
	if !isReservedIpStr(obip) {
		tp.enable = true
	}

	tp.server = true
	tp.localIP = obip
	return tp
}

func NewDiectUdpTransportClient(srvip string) *DirectUdpTransport {
	tp := &DirectUdpTransport{}
	obip := srvip
	if !isReservedIpStr(obip) {
		tp.enable = true
	}

	tp.server = false
	tp.peerIP = srvip
	tp.localIP = getOutboundIp()
	return tp
}

func (this *DirectUdpTransport) init() bool {
	this.readyReadNoticeChan = make(chan CommonEvent, mpcsz)
	this.readyReadDataChan = make(chan UdpReadyReadEvent, mpcsz)
	this.readyReadDataChanType = reflect.TypeOf(this.readyReadDataChan)

	if this.server {
		return this.initServer()
	} else {
		return this.initServer()
	}
}

func (this *DirectUdpTransport) initServer() bool {
	udpSrv, err := net.ListenPacket("udp", ":18588")
	if err != nil {
		log.Fatalln(err, udpSrv)
	}
	info.Println("Listen UDP:", this.udpSrv.LocalAddr().String())
	this.udpSrv = udpSrv
	return true
}

func (this *DirectUdpTransport) initClient() bool {
	// connect to server?
	return true
}

func (this *DirectUdpTransport) serve() {
	if this.server {
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
			debug.Println(rdn, addr, err)
		} else {
			evt := UdpReadyReadEvent{addr, buf[0:rdn], rdn}
			this.readyReadNoticeChan <- CommonEvent{reflect.TypeOf(evt), reflect.ValueOf(evt)}
		}
	}
}

func (this *DirectUdpTransport) getEventData(evt CommonEvent) ([]byte, int, interface{}) {
	devt := evt.v.Interface().(UdpReadyReadEvent)
	return devt.buf, devt.size, devt.addr
}

func (this *DirectUdpTransport) serveClient() {

}

func (this *DirectUdpTransport) getReadyReadDataChanType() reflect.Type {
	return this.readyReadDataChanType
}

func (this *DirectUdpTransport) getReadyReadChannel() chan CommonEvent {
	return this.readyReadNoticeChan
}

func (this *DirectUdpTransport) getConn() interface{} {
	return nil
}

func (this *DirectUdpTransport) sendData(buf []byte, size int, uaddr net.Addr) int {
	if this.server {
		return this.sendDataServer(buf, size, uaddr)
	} else {
		return this.sendDataClient(buf, size)
	}
}

func (this *DirectUdpTransport) sendDataServer(buf []byte, size int, uaddr net.Addr) int {
	wrn, err := this.udpSrv.WriteTo(buf[:size], uaddr)
	if err != nil {
		errl.Println(err, wrn)
	}
	return wrn
}

func (this *DirectUdpTransport) sendDataClient(buf []byte, size int) int {
	outboundip := this.peerIP
	uaddr, err := net.ResolveUDPAddr("udp", outboundip)
	if err != nil {
		errl.Println(err, uaddr)
	}

	// replace with? this.sendDataServer(buf, size, uaddr)
	wrn, err := this.udpSrv.WriteTo(buf[:size], uaddr)
	if err != nil {
		errl.Println(err, wrn)
	}
	return wrn
}

type ToxMessageTransport struct {
	TransportBase
}

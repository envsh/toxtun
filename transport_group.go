/*
transport只需要一个实例，但需要处理不同的channel。
在此做不同transport实例的管理与不同channel的分发。
*/
package main

import (
	"log"
	"reflect"
	"strings"
	"sync"

	"github.com/kitech/go-toxcore"
)

type TransportGroup struct {
	TransportBase
	t             *tox.Tox
	udptp         *DirectUdpTransport
	toxlossytp    *ToxLossyTransport
	toxlosslesstp *ToxLosslessTransport
	ethereumtp    *EthereumTransport
	// kcptp         *KcpTransport

	tps    []Transport
	chans  map[uint32]*Channel // chid/convid => Channel, for dispatch
	chpool *ChannelPool

	shutWG    sync.WaitGroup
	quitPollC chan bool
}

func NewTransportGroup(t *tox.Tox, server bool, chpool *ChannelPool) *TransportGroup {
	this := &TransportGroup{}
	this.name_ = "group"
	this.t = t
	this.isServer = server
	this.tps = make([]Transport, 0)
	this.chans = make(map[uint32]*Channel)
	this.chpool = NewChannelPool()
	this.chpool = chpool
	this.quitPollC = make(chan bool, 1)

	this.init()
	go this.serve()
	return this
}

func (this *TransportGroup) init() bool {
	this.readyReadNoticeChan = make(chan CommonEvent, mpcsz)
	this.readyReadDataChanType = reflect.TypeOf(this.readyReadNoticeChan)
	this.initTransports()
	return true
}
func (this *TransportGroup) serve() {
	this.PollForever()
}
func (this *TransportGroup) shutdown() {
	this.quitPollC <- true
	this.shutWG.Wait()
}

func (this *TransportGroup) getReadyReadChan() <-chan CommonEvent {
	return this.readyReadNoticeChan
}
func (this *TransportGroup) getReadyReadChanType() reflect.Type {
	return this.readyReadDataChanType
}
func (this *TransportGroup) getEventData(evt CommonEvent) ([]byte, int, interface{}) {
	gevt := evt.v.Interface().(GroupReadyReadEvent)
	return gevt.tp.getEventData(gevt.evt)
}
func (this *TransportGroup) sendData(data string, to string) error {
	if this.isServer {
		tos := strings.Split(to, ",")
		if len(tos) != len(this.tps) {
			log.Println(lwarningp, "wtf", tos, to)
		}

		for idx, tp := range this.tps {
			err := tp.sendData(data, tos[idx])
			if err != nil {
				log.Println(err)
			}
		}
	} else {
		tos := strings.Split(to, ",")
		if len(tos) != len(this.tps) {
			log.Println(lwarningp, "wtf", tos, to)
		}

		for idx, tp := range this.tps {
			err := tp.sendData(data, tos[idx])
			if err != nil {
				log.Println(err)
			}
		}
	}
	return nil
}
func (this *TransportGroup) localVirtAddr() string {
	vaddrs := make([]string, 0)
	for _, tp := range this.tps {
		vaddrs = append(vaddrs, tp.localVirtAddr())
	}
	this.localVirtAddr_ = strings.Join(vaddrs, ",")
	log.Println(this.localVirtAddr_)
	return this.localVirtAddr_
}

//////
func (this *TransportGroup) initTransports() {
	if this.isServer {
		this.udptp = NewDirectUdpTransport()
	} else {
		this.udptp = NewDirectUdpTransportClient("") // TODO when fill the field?
	}
	if true {
		this.toxlossytp = NewToxLossyTransport(this.t)
	} else {
		this.toxlosslesstp = NewToxLosslessTransport(this.t)
		log.Panicln("not supported")
	}
	// this.ethereumtp = NewEthereumTransport(this.isServer)

	//
	// this.tps = append(this.tps, this.udptp)
	this.tps = append(this.tps, this.toxlossytp)
	// this.tps = append(this.tps, this.toxlosslesstp)
	// this.tps = append(this.tps, this.ethereumtp)
}

func (this *TransportGroup) addChannel(ch *Channel) {
	if this.isServer {
		this.chpool.putServer(ch)
	} else {
		this.chpool.putClientLacks(ch)
		this.chpool.putClient(ch)
	}
}

func (this *TransportGroup) delChannel(ch *Channel) {
	if this.isServer {
		this.chpool.rmServer(ch)
	} else {
		this.chpool.rmClient(ch)
	}
}

type GroupReadyReadEvent struct {
	tp  Transport
	evt CommonEvent
}

// should block
func (this *TransportGroup) PollForever() {

	// dispatch
	processTransportReadyRead := func(tp Transport, evt CommonEvent) {
		if false {
			this.readyReadNoticeChan <- newCommonEvent(GroupReadyReadEvent{tp, evt})
		}

		//
		buf, size, extra := tp.getEventData(evt)
		unused(buf, size, extra)
		/*
			// how dispatch
				for x, ch := range this.chanpool.pool {

				}
		*/

		/*
			var conv uint32
			// kcp包前4字段为conv，little hacky
			if len(buf) < 4 {
				log.Println(lerrorp, "wtf")
			}
			conv = binary.LittleEndian.Uint32(buf)
			ch := this.chpool.pool2[conv]
			if ch == nil {
				log.Println(lerrorp, "maybe has some problem", conv)
			} else {
		*/
		if this.isServer {
			switch tp.(type) {
			case *DirectUdpTransport: // update channel's peerAddr
				/*
					pvas := strings.Split(ch.peerVirtAddr, ",")
					newaddr := extra.(net.Addr).String()
					if pvas[0] != newaddr {
						log.Println(pvas[0], "->", newaddr, ch.peerVirtAddr)
						pvas[0] = newaddr
						ch.peerVirtAddr = strings.Join(pvas, ",")
					}
				*/
			}
		}

		// kcptp := ch.tp.(*KcpTransport)
		// kcptp.processSubTransport(newCommonEvent(GroupReadyReadEvent{tp, evt}))
		// kcptp.InputChan <- newCommonEvent(GroupReadyReadEvent{tp, evt})
		//}

		this.readyReadNoticeChan <- newCommonEvent(GroupReadyReadEvent{tp, evt})
	}

	this.shutWG.Add(1)
	for {
		select {
		case evt := <-this.udptp.getReadyReadChan():
			processTransportReadyRead(this.udptp, evt)
		case evt := <-this.toxlossytp.getReadyReadChan():
			processTransportReadyRead(this.toxlossytp, evt)
			// case evt := <-this.ethereumtp.getReadyReadChan():
			//	processTransportReadyRead(this.ethereumtp, evt)
		case <-this.quitPollC:
			this.udptp.shutdown()
			this.toxlossytp.shutdown()
			this.shutWG.Done()
			return
		}
	}
}

// simple factory
func (this *TransportGroup) NewKcpTransport(ch *Channel) *KcpTransport {
	t := this.t
	server := this.isServer

	// TODO ???
	// this.addChannel(ch)

	return NewKcpTransport(t, ch, server, this)
}

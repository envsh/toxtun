/*
transport只需要一个实例，但需要处理不同的channel。
在此做不同transport实例的管理与不同channel的分发。
*/
package main

type TransportGroup struct {
	udptp         *DirectUdpTransport
	toxlossytp    *ToxLossyTransport
	toxlosslesstp *ToxLosslessTransport
	ethereumtp    *EthereumTransport
	// kcptp         *KcpTransport

	tps      []Transport
	chans    map[uint32]*Channel // chid/convid => Channel, for dispatch
	chanpool *ChannelPool
}

func newTransportGroup() *TransportGroup {
	this := &TransportGroup{}
	this.tps = make([]Transport, 0)
	this.chans = make(map[uint32]*Channel)
	this.chanpool = NewChannelPool()

	return this
}

func (*TransportGroup) addChannel(ch *Channel) {

}

func (*TransportGroup) delChannel(ch *Channel) {

}

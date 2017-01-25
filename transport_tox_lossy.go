package main

import (
	"log"
	"reflect"

	"gopp"

	"github.com/kitech/go-toxcore"
)

type ToxLossyTransport struct {
	TransportBase
	tox *tox.Tox
}

func NewToxLossyTransport(t *tox.Tox) *ToxLossyTransport {
	if false {
		log.Println(t)
	}
	this := &ToxLossyTransport{}
	this.tox = t

	t.CallbackFriendConnectionStatus(this.onToxnetFriendConnectionStatus, this)
	t.CallbackFriendMessage(this.onToxnetFriendMessage, this)
	// t.CallbackFriendLosslessPacketAdd(this.onToxnetFriendLosslessPacket, this)
	t.CallbackFriendLossyPacketAdd(this.onToxnetFriendLossyPacket, this)

	return this
}

func (this *ToxLossyTransport) init() bool {
	return true
}
func (this *ToxLossyTransport) serve() {

}
func (this *ToxLossyTransport) getReadyReadChan() <-chan CommonEvent {
	return nil
}
func (this *ToxLossyTransport) getReadyReadChanType() reflect.Type {
	return reflect.TypeOf("123")
}
func (this *ToxLossyTransport) getEventData(evt CommonEvent) ([]byte, int, interface{}) {
	return nil, 0, nil
}

func (this *ToxLossyTransport) sendData(data string) error {
	return nil
}

/////
func (this *ToxLossyTransport) onToxnetFriendConnectionStatus(t *tox.Tox, friendNumber uint32, status int, userData interface{}) {
}

func (this *ToxLossyTransport) onToxnetFriendMessage(t *tox.Tox, friendNumber uint32, message string, userData interface{}) {
	log.Println(friendNumber, len(message), gopp.StrSuf(message, 52))
}

func (this *ToxLossyTransport) onToxnetFriendLossyPacket(t *tox.Tox, friendNumber uint32, message string, userData interface{}) {
	log.Println(friendNumber, len(message), gopp.StrSuf(message, 52))
}

func (this *ToxLossyTransport) onToxnetFriendLosslessPacket(t *tox.Tox, friendNumber uint32, message string, userData interface{}) {
	log.Println(friendNumber, len(message), gopp.StrSuf(message, 52))
}

////////////
func (this *ToxLossyTransport) FriendSendMessage(friendId string, message string) (uint32, error) {
	friendNumber, err := this.tox.FriendByPublicKey(friendId)
	if err != nil {
		return 0, err
	}
	return this.tox.FriendSendMessage(friendNumber, message)
}

func (this *ToxLossyTransport) FriendSendLossyPacket(friendId string, message string) error {
	friendNumber, err := this.tox.FriendByPublicKey(friendId)
	if err != nil {
		return err
	}
	return this.tox.FriendSendLossyPacket(friendNumber, message)
}

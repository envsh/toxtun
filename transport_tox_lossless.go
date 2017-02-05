package main

import (
	"log"
	"reflect"

	"gopp"

	"github.com/kitech/go-toxcore"
)

type ToxLosslessTransport struct {
	TransportBase
	tox *tox.Tox
}

func NewToxLosslessTransport(t *tox.Tox) *ToxLosslessTransport {
	if false {
		log.Println(t)
	}
	this := &ToxLosslessTransport{}
	this.name_ = "toxlossless"
	this.tox = t
	this.lossy = false

	t.CallbackFriendConnectionStatus(this.onToxnetFriendConnectionStatus, this)
	t.CallbackFriendMessage(this.onToxnetFriendMessage, this)
	t.CallbackFriendLosslessPacketAdd(this.onToxnetFriendLosslessPacket, this)
	// t.CallbackFriendLossyPacketAdd(this.onToxnetFriendLossyPacket, this)

	return this
}

func (this *ToxLosslessTransport) init() bool {
	return true
}
func (this *ToxLosslessTransport) serve() {

}
func (this *ToxLosslessTransport) getReadyReadChan() <-chan CommonEvent {
	return nil
}
func (this *ToxLosslessTransport) getReadyReadChanType() reflect.Type {
	return reflect.TypeOf("123")
}
func (this *ToxLosslessTransport) getEventData(evt CommonEvent) ([]byte, int, interface{}) {
	return nil, 0, nil
}

/////
func (this *ToxLosslessTransport) onToxnetFriendConnectionStatus(t *tox.Tox, friendNumber uint32, status int, userData interface{}) {
}

func (this *ToxLosslessTransport) onToxnetFriendMessage(t *tox.Tox, friendNumber uint32, message string, userData interface{}) {
	log.Println(friendNumber, len(message), gopp.StrSuf(message, 52))
}

func (this *ToxLosslessTransport) onToxnetFriendLossyPacket(t *tox.Tox, friendNumber uint32, message string, userData interface{}) {
	log.Println(friendNumber, len(message), gopp.StrSuf(message, 52))
}

func (this *ToxLosslessTransport) onToxnetFriendLosslessPacket(t *tox.Tox, friendNumber uint32, message string, userData interface{}) {
	log.Println(friendNumber, len(message), gopp.StrSuf(message, 52))
}

////////////
func (this *ToxLosslessTransport) FriendSendMessage(friendId string, message string) (uint32, error) {
	friendNumber, err := this.tox.FriendByPublicKey(friendId)
	if err != nil {
		return 0, err
	}
	return this.tox.FriendSendMessage(friendNumber, message)
}

func (this *ToxLosslessTransport) FriendSendLossyPacket(friendId string, message string) error {
	friendNumber, err := this.tox.FriendByPublicKey(friendId)
	if err != nil {
		return err
	}
	return this.tox.FriendSendLossyPacket(friendNumber, message)
}

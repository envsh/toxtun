package main

import (
	"bytes"
	"gopp"
	"log"
	"sync"
	"time"

	tox "github.com/TokTok/go-toxcore-c"
)

type Inputer interface {
	Input([]byte) error
}
type Dispatcher struct {
	tox    *tox.Tox
	tptype int

	inputers sync.Map // symbol(friendNumber|public_key) => Inputer
}

const (
	TPTYPE_NONE     = 0
	TPTYPE_MESSAGE  = 1
	TPTYPE_LOSSY    = 2
	TPTYPE_LOSSLESS = 3
)

func NewDispatcher(tox *tox.Tox, tptype int) *Dispatcher {
	this := &Dispatcher{}
	this.tox = tox
	this.tptype = tptype

	// callbacks
	t := tox
	t.CallbackFriendMessage(this.onToxnetFriendMessage, nil)
	t.CallbackFriendLossyPacket(this.onToxnetFriendLossyPacket, nil)
	t.CallbackFriendLosslessPacket(this.onToxnetFriendLosslessPacket, nil)
	return this
}

func (this *Dispatcher) RegisterInputer(symbol interface{}, inputer Inputer) {
	if _, ok := this.inputers.LoadOrStore(symbol, inputer); ok {
		log.Println("already exist", symbol)
	}
}

func (this *Dispatcher) dispatch(friendNumber uint32, message string) {
	if inputerx, ok := this.inputers.Load(friendNumber); ok {
		fname, _ := this.tox.FriendGetName(friendNumber)
		if fname == "prnuid.ef55c74a-08ae-4640-a950-10d6b24b94aa.3FEBE" {
			return
		}
		log.Println("dispatch friend message", friendNumber, message)
		log.Panicln("hhh", gopp.Retn(this.tox.FriendGetName(friendNumber)))
		inputer := inputerx.(Inputer)
		err := inputer.Input([]byte(message))
		gopp.ErrPrint(err, friendNumber)
	} else {
		log.Println("Inputer not found", friendNumber)
	}
}

func (this *Dispatcher) onToxnetFriendMessage(t *tox.Tox, friendNumber uint32, message string, userData interface{}) {
	debug.Println(friendNumber, len(message), gopp.StrSuf(message, 52))
	friendId, err := this.tox.FriendGetPublicKey(friendNumber)
	if err != nil {
		errl.Println(err, friendId)
	}
	if this.tptype == TPTYPE_MESSAGE {
		this.dispatch(friendNumber, message)
	}
}

func (this *Dispatcher) onToxnetFriendLossyPacket(t *tox.Tox, friendNumber uint32, message string, userData interface{}) {
	debug.Println(friendNumber, len(message), gopp.StrSuf(message, 52), time.Now().String())
	buf := bytes.NewBufferString(message).Bytes()
	if buf[0] == 254 {
		data := buf[1:]
		// this.kcpInputChan <- ClientReadyReadEvent{nil, data, len(data), false}
		// debug.Println("tox->kcp:", len(buf), gopp.StrSuf(string(buf), 52))
		if this.tptype == TPTYPE_LOSSY {
			this.dispatch(friendNumber, string(data))
		}
	} else {
		info.Println("unknown message:", buf[0])
	}
}

func (this *Dispatcher) onToxnetFriendLosslessPacket(t *tox.Tox, friendNumber uint32, message string, userData interface{}) {
	debug.Println(friendNumber, len(message), gopp.StrSuf(message, 52))
	buf := bytes.NewBufferString(message).Bytes()
	if buf[0] == 191 {
		data := buf[1:]
		if this.tptype == TPTYPE_LOSSLESS {
			this.dispatch(friendNumber, string(data))
		}
	} else {
		info.Println("unknown message:", buf[0])
	}
}

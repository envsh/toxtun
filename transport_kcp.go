package main

import (
	"encoding/base64"
	"log"
	// "math"
	"reflect"
	"time"

	"gopp"

	"github.com/kitech/go-toxcore"
)

type KcpTransport struct {
	TransportBase
	subtps []Transport
	tp     Transport
	// chpool *ChannelPool
	t  *tox.Tox
	ch *Channel

	kcp               *KCP // from Channel.kcp
	kcpPollChan       chan KcpPollEvent
	kcpNextUpdateWait int
	kcpCheckCloseChan chan KcpCheckCloseEvent
}

func NewKcpTransport(t *tox.Tox, ch *Channel, server bool) *KcpTransport {
	if false {
		log.Println(1)
	}
	this := &KcpTransport{}
	this.ch = ch
	this.server = server

	this.tp = NewToxLossyTransport(t)
	conv := uint32(123456)
	conv = ch.conv
	this.kcp = NewKCP(conv, this.onKcpOutput, nil)
	this.kcp.SetMtu(tunmtu)
	if kcp_mode == "fast" {
		this.kcp.WndSize(128, 128)
		this.kcp.NoDelay(1, 10, 2, 1)
	}

	this.kcpPollChan = make(chan KcpPollEvent, mpcsz)
	this.kcpCheckCloseChan = make(chan KcpCheckCloseEvent, mpcsz)

	go this.serve()
	return this
}

func (this *KcpTransport) init() bool {
	return true
}
func (this *KcpTransport) serve() {
	log.Println(ldebugp)
	stop := false
	go func() {
		for {
			time.Sleep(2000 * time.Millisecond)
			this.kcpPollChan <- KcpPollEvent{}
			// this.serveKcp()
		}
	}()

	go func() {
		for {
			time.Sleep(1000 * time.Millisecond)
			this.kcpCheckCloseChan <- KcpCheckCloseEvent{}
		}
	}()

	// like event handler
	for !stop {
		select {
		case <-this.kcpPollChan:
			this.serveKcp()
		case evt := <-this.tp.getReadyReadChan():
			this.processSubTransport(evt)
			// processSubTransport
		}
	}
	log.Println(linfop)
}
func (this *KcpTransport) getReadyReadChan() <-chan CommonEvent {
	return nil
}
func (this *KcpTransport) getReadyReadChanType() reflect.Type {
	return reflect.TypeOf("123")
}
func (this *KcpTransport) getEventData(evt CommonEvent) ([]byte, int, interface{}) {
	return nil, 0, nil
}
func (this *KcpTransport) sendData(data string, to string) error {
	log.Println(ldebugp)
	// n := this.kcp.Input([]byte(data))
	n := this.kcp.Send([]byte(data))
	// log.Println(ldebugp, n, len(data), len([]byte(data)), IKCP_OVERHEAD)
	switch n {
	case -1:
		log.Println(lerrorp, n)
	}
	return nil
}

/////
// TODO 计算kcpNextUpdateWait的逻辑优化
func kcp_poll(pool map[int]*Channel) (chks []*Channel, nxtss []uint32) {
	for _, ch := range pool {
		unused(ch)
		/*
			if ch.kcp == nil {
				continue
			}
			curts := uint32(iclock())
			rts := ch.kcp.Check(curts)
			if rts == curts {
				nxtss = append(nxtss, 10)
				chks = append(chks, ch)
			} else {
				nxtss = append(nxtss, rts-curts)
			}
		*/
		panic(123)
	}

	return
}

func (this *KcpTransport) serveKcp() {
	{
		zbuf := make([]byte, 0)
		kcp := this.kcp
		kcp.Update(uint32(iclock()))

		n := kcp.Recv(zbuf)
		switch n {
		case -3:
			this.processKcpReadyRead(nil)
		case -2: // just empty kcp recv queue
			// errl.Println("kcp recv internal error:", n, this.kcp.PeekSize())
		case -1: // EAGAIN
		default:
			errl.Println("unknown recv:", n)
		}

	}
	/*
		log.Println(ldebugp)
		zbuf := make([]byte, 0)
		if true {
			chks, nxtss := kcp_poll(this.chpool.pool)

			mints := gopp.MinU32(nxtss)
			if mints > 10 && mints != math.MaxUint32 {
				this.kcpNextUpdateWait = int(mints)
				return
			} else {
				this.kcpNextUpdateWait = 10
			}

			for _, ch := range chks {
					if ch.kcp == nil {
						continue
					}

					ch.kcp.Update(uint32(iclock()))

					n := ch.kcp.Recv(zbuf)
					switch n {
					case -3:
						this.processKcpReadyRead(ch)
					case -2: // just empty kcp recv queue
						// errl.Println("kcp recv internal error:", n, this.kcp.PeekSize())
					case -1: // EAGAIN
					default:
						errl.Println("unknown recv:", n)
					}
			}
		}
	*/
}
func (this *KcpTransport) kcpCheckClose() {
	/*
			closed := make([]*Channel, 0)
			for _, ch := range this.chpool.pool {
					if ch.kcp == nil {
						continue
					}

					if ch.server_socket_close == true && ch.server_kcp_close == false {
						cnt := ch.kcp.WaitSnd()
						if cnt == 0 {
							debug.Println("channel empty:", ch.chidcli, ch.chidsrv, ch.conv)
							ch.server_kcp_close = true
							closed = append(closed, ch)
						}
					}

			}

		for _, ch := range closed {
			if false {
				log.Println(ch)
			}
			// this.promiseChannelClose(ch)
		}
	*/
}

func (this *KcpTransport) onKcpOutput(buf []byte, size int, extra interface{}) {
	if size <= 0 {
		// 如果总是出现，并且不影响程序运行，那么也就不是bug了
		// info.Println("wtf")
		return
	}
	debug.Println(len(buf), "//", size, "//", string(gopp.SubBytes(buf, 52)))
	// ch := extra.(*Channel)

	msg := string([]byte{254}) + string(buf[:size])
	var err error
	err = this.tp.sendData(string(buf[:size]), this.ch.toxid)
	// err := this.FriendSendLossyPacket(ch.toxid, msg)
	// msg := string([]byte{191}) + string(buf[:size])
	// err := this.tox.FriendSendLosslessPacket(0, msg)
	if err != nil {
		debug.Println(err)
	} else {
		debug.Println("kcp->tox:", len(msg), time.Now().String())
	}

	// multipath-udp backend
}

func (this *KcpTransport) processKcpReadyRead(ch *Channel) {
	/*
		if ch.conn == nil {
			errl.Println("Not Connected:", ch.chidsrv, ch.chidcli)
			// return
		}
	*/
	kcp := this.kcp

	buf := make([]byte, kcp.PeekSize())
	n := kcp.Recv(buf)

	if len(buf) != n {
		errl.Println("Invalide kcp recv data")
	}

	pkt := parsePacket(buf)
	if pkt.isconnack() {
	} else if pkt.isdata() {
		// ch := this.chpool.pool[pkt.chidsrv]
		ch = this.ch
		debug.Println("processing channel data:", ch.chidsrv, len(pkt.data), gopp.StrSuf(pkt.data, 52))
		buf, err := base64.StdEncoding.DecodeString(pkt.data)
		if err != nil {
			errl.Println(err)
		}

		wn, err := ch.conn.Write(buf)
		if err != nil {
			errl.Println(err)
		}
		debug.Println("kcp->srv:", wn)
		appevt.Trigger("reqbytes", wn, len(buf)+25)
	} else {
	}

}

func (this *KcpTransport) processSubTransport(evt CommonEvent) {
	s := evt.v.Interface().(string)
	n := this.kcp.Input([]byte(s))
	switch n {
	case -1, -10, -2, -3:
		log.Println(ldebugp, n)
	}
}

/*
KCP的全双工:
write: Send -> Output -> TP
read: Input -> Recv -> TP
*/

package main

import (
	"encoding/base64"
	"fmt"
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

	InputChan chan CommonEvent // for skip lock

}

func NewKcpTransport(t *tox.Tox, ch *Channel, server bool, tp Transport) *KcpTransport {
	if false {
		log.Println(1)
	}
	this := &KcpTransport{}
	this.name_ = "kcp"
	this.ch = ch
	this.isServer = server

	if false {
		this.tp = NewToxLossyTransport(t)
		if server {
			this.tp = NewDirectUdpTransport()
		} else {
			this.tp = NewDirectUdpTransportClient(ch.ip)
		}
		this.tp = NewEthereumTransport(server)
	} else {
		this.tp = tp
	}

	conv := ch.conv
	this.kcp = NewKCP(conv, this.onKcpOutput, nil)
	this.kcp.SetMtu(tunmtu)
	if kcp_mode == "fast" {
		wnsz := 128 // 32(default), 128
		this.kcp.WndSize(wnsz, wnsz)
		this.kcp.NoDelay(1, 10, 2, 1)
	}
	setKCPMode(1, this.kcp) // 0,1,2

	this.kcpPollChan = make(chan KcpPollEvent, mpcsz)
	this.kcpCheckCloseChan = make(chan KcpCheckCloseEvent, mpcsz)

	this.InputChan = make(chan CommonEvent, mpcsz)

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
			// time.Sleep(2000 * time.Millisecond)
			time.Sleep(1 * time.Millisecond)
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
		case evt := <-this.InputChan:
			this.processSubTransport(evt)
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
	n := this.kcp.Send([]byte(data))
	switch {
	case n < 0:
		log.Println(lerrorp, n)
		return anyerror(n)
	case n == 0: // ok
		// this.kcpPollChan <- KcpPollEvent{}
		if this.isServer {
			log.Println(ldebugp, "srv->kcp:", len(data))
		} else {
			log.Println(ldebugp, "cli->kcp:", len(data))
		}
	case n > 0: // no this ret
	}
	return nil
}
func (this *KcpTransport) sendBufferFull() bool {
	if uint32(this.kcp.WaitSnd()) > this.kcp.snd_wnd*5 {
		return true
	}
	return false
}
func (this *KcpTransport) localVirtAddr() string {
	return this.tp.localVirtAddr()
}
func (this *KcpTransport) name() string { return this.name_ }

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
	if this == nil {
		log.Println(lerrorp, "already left")
		return
	}
	{
		kcp := this.kcp
		// kcp.Update(uint32(iclock2()))
		kcp.Update()

		n := kcp.Recv(nil)
		switch n {
		case -3: // available size  > 0
			this.processKcpReadyRead(nil)
		case -2: // just empty kcp recv queue
			// errl.Println("kcp recv internal error:", n, this.kcp.PeekSize())
		case -1: // EAGAIN
		default:
			log.Println(lwarningp, "unknown recv:", n)
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
		// log.Println(lwarningp, "wtf")
		return
	}
	log.Println(ldebugp, len(buf), "/", size, "/", string(gopp.SubBytes(buf, 52)))

	var err error
	err = this.tp.sendData(string(buf[:size]), this.ch.peerVirtAddr)
	if err != nil {
		log.Println(lerrorp, err)
	} else {
		totpname := this.tp.name()
		log.Println(ldebugp, fmt.Sprintf("kcp->%s:", totpname), size)
	}

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
		log.Println(lerrorp, "Invalide kcp recv data")
	}

	pkt := parsePacket(buf)
	if pkt == nil {
		log.Println(lerrorp, "packge broken")
		return
	}
	if pkt.isconnack() {
	} else if pkt.isdata() {
		// ch := this.chpool.pool[pkt.chidsrv]
		ch = this.ch
		log.Println(ldebugp, "processing channel data:", ch.chidsrv, len(pkt.data), gopp.StrSuf(pkt.data, 52))
		buf, err := base64.StdEncoding.DecodeString(pkt.data)
		if err != nil {
			log.Println(lerrorp, err)
		}

		// 这么检测应该还是有可能crash
		if ch.client_socket_close {
			log.Println(lerrorp, "client socket is closed.", ch.conv)
		} else {
			wn, err := ch.conn.Write(buf) // crash here SIGPIPE
			if err != nil {
				log.Println(lerrorp, err)
			}
			if this.isServer {
				log.Println(ldebugp, "kcp->srv:", wn)
			} else {
				log.Println(ldebugp, "kcp->cli:", wn)
			}
			appevt.Trigger("reqbytes", wn, len(buf)+25)
		}
	} else {
	}

}

func (this *KcpTransport) processSubTransport(evt CommonEvent) {
	// s := evt.v.Interface().(string)
	data, sz, x := this.tp.getEventData(evt)
	// n := this.kcp.Input(data)
	n := this.kcp.Input(data, true)
	switch {
	case n < 0:
		log.Println(lerrorp, n, sz, x)
		switch n {
		case -10: // convid not match
		}
	case n == 0: // ok
		// this.kcpPollChan <- KcpPollEvent{} // why slow down?
		switch eval := evt.v.Interface().(type) {
		case GroupReadyReadEvent:
			fromtpname := eval.tp.name()
			log.Println(ldebugp, fromtpname+"->kcp:", len(data))
		default:
			fromtpname := this.tp.name()
			log.Println(ldebugp, fromtpname+"->kcp:", len(data))
		}
	}
}

func setKCPMode(mode int, kcp *KCP) {
	if mode == 0 {
		// 默认模式
		kcp.NoDelay(0, 10, 0, 0)
	} else if mode == 1 {
		// 普通模式，关闭流控等
		kcp.NoDelay(0, 10, 0, 1)
	} else {
		// 启动快速模式
		// 第二个参数 nodelay-启用以后若干常规加速将启动
		// 第三个参数 interval为内部处理时钟，默认设置为 10ms
		// 第四个参数 resend为快速重传指标，设置为2
		// 第五个参数 为是否禁用常规流控，这里禁止
		kcp.NoDelay(1, 10, 2, 1)
	}
}

/*
KCP的全双工:
write: Send -> Output -> TP
read: Input -> Recv -> TP

server:
socket(read) -> KCP(INPUT) -> APP(RECV)
app(send) -> KCP(Send) ->socket(write)

client:
socket(read) -> KCP(INPUT) -> APP(RECV)
app(send) -> KCP(Send) ->socket(write)

capp  <-> cKCP <-> csocket <-> internet <-> ssocket <-> sKCP <-> sapp

*/

package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"log"
	"math"
	"reflect"
	"sync"
	"time"

	"gopp"

	"github.com/kitech/go-toxcore"
)

var kcpConv uint32 = math.MaxUint32 / 2

type KcpTransport struct {
	TransportBase
	subtps []Transport
	tp     Transport
	// chpool *ChannelPool
	t *tox.Tox
	// ch *Channel
	peerVirtAddr string

	kcp               *KCP // from Channel.kcp
	kcpPollChan       chan KcpPollEvent
	kcpNextUpdateWait int
	kcpCheckCloseChan chan KcpCheckCloseEvent

	InputChan chan CommonEvent // for skip lock

	shutWG    sync.WaitGroup
	quitTickC chan bool
	quitCtrlC chan bool
}

func NewKcpTransport2(t *tox.Tox, pvaddr string, server bool, tp Transport) *KcpTransport {
	if false {
		log.Println(1)
	}
	this := &KcpTransport{}
	this.name_ = "kcp"
	this.isServer = server
	this.peerVirtAddr = pvaddr

	if false {
		this.tp = NewToxLossyTransport(t)
		if server {
			this.tp = NewDirectUdpTransport()
		} else {
			// this.tp = NewDirectUdpTransportClient(ch.ip)
		}
		this.tp = NewEthereumTransport(server)
	} else {
		this.tp = tp
	}

	// conv := ch.conv
	conv := kcpConv
	this.kcp = NewKCP(conv, this.onKcpOutput, nil)
	this.kcp.SetMtu(tunmtu)
	if kcp_mode == "fast" {
		wnsz := 128 // 32(default), 128
		this.kcp.WndSize(wnsz, wnsz)
		this.kcp.NoDelay(1, 10, 2, 1)
	}
	setKCPMode(1, this.kcp) // 0,1,2,3

	this.readyReadNoticeChan = make(chan CommonEvent, mpcsz)
	this.kcpPollChan = make(chan KcpPollEvent, mpcsz)
	this.kcpCheckCloseChan = make(chan KcpCheckCloseEvent, mpcsz)

	this.InputChan = make(chan CommonEvent, mpcsz)

	this.quitTickC = make(chan bool, 1)
	this.quitCtrlC = make(chan bool, 1)

	go this.serve()

	return this
}

func NewKcpTransport(t *tox.Tox, ch *Channel, server bool, tp Transport) *KcpTransport {
	if false {
		log.Println(1)
	}
	this := &KcpTransport{}
	this.name_ = "kcp"
	// this.ch = ch
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
	setKCPMode(1, this.kcp) // 0,1,2,3

	this.kcpPollChan = make(chan KcpPollEvent, mpcsz)
	this.kcpCheckCloseChan = make(chan KcpCheckCloseEvent, mpcsz)

	this.InputChan = make(chan CommonEvent, mpcsz)

	this.quitTickC = make(chan bool, 1)
	this.quitCtrlC = make(chan bool, 1)

	go this.serve()
	return this
}

func (this *KcpTransport) init() bool {
	return true
}
func (this *KcpTransport) serve() {
	log.Println(ldebugp)
	go func() {
		this.shutWG.Add(1)
		// tickKcpPollC := time.Tick(2000 * time.Millisecond)
		tickKcpPollC := time.Tick(200 * time.Millisecond) // slow
		// tickKcpPollC := time.Tick(20 * time.Millisecond) // medium
		// tickKcpPollC := time.Tick(2 * time.Millisecond) // crazy
		tickKcpCheckC := time.Tick(1000 * time.Millisecond)

		for {
			select {
			case <-tickKcpPollC:
				this.kcpPollChan <- KcpPollEvent{}
				// this.serveKcp()
			case <-tickKcpCheckC:
				this.kcpCheckCloseChan <- KcpCheckCloseEvent{}
			case <-this.quitTickC:
				this.shutWG.Done()
				return
			}
		}
	}()

	// like event handler
	this.shutWG.Add(1)
	for {
		select {
		case <-this.kcpPollChan:
			this.serveKcp()
		case evt := <-this.tp.getReadyReadChan():
			this.processSubTransport(evt)
			// processSubTransport
		case evt := <-this.InputChan:
			log.Println(lerrorp, "depcreated", &evt)
			// this.processSubTransport(evt)
		case <-this.quitCtrlC:
			goto end
		}
	}
end:
	log.Println(linfop, "ctrl routine finished")
	this.shutWG.Done()
}
func (this *KcpTransport) shutdown() {
	log.Println(ldebugp, "closing kcp transport...")
	switch this.tp.(type) {
	case *TransportGroup:
	default:
		this.tp.shutdown()
	}

	this.quitTickC <- true
	this.quitCtrlC <- true

	log.Println(linfop, "waiting sub goroutines finish...")
	this.shutWG.Wait()
	log.Println(linfop, "all sub goroutines finished.")
}
func (this *KcpTransport) getReadyReadChan() <-chan CommonEvent {
	return this.readyReadNoticeChan
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
	// err = this.tp.sendData(string(buf[:size]), this.ch.peerVirtAddr)
	err = this.tp.sendData(string(buf[:size]), this.peerVirtAddr)
	if err != nil {
		log.Println(lerrorp, err)
	} else {
		totpname := this.tp.name()
		log.Println(ldebugp, fmt.Sprintf("kcp->%s:", totpname), size)
	}

}

func (this *KcpTransport) processKcpReadyRead(*Channel) {
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
		log.Println(lerrorp, "Invalide kcp recv data", len(buf), n)
	}
	if len(buf) != n && n > 0 {
		buf = buf[:n]
	}

	bufrd := bytes.NewBuffer(buf[:n])
	for {
		buf := make([]byte, tunmtu)
		rn, err := bufrd.Read(buf)
		if err != nil {
			break
		}
		buf = buf[:rn]

		comevt := CommonEvent{reflect.TypeOf(buf), reflect.ValueOf(buf)}
		this.readyReadNoticeChan <- comevt
		if this.isServer {
			log.Println(ldebugp, "kcp->srv:", rn)
		} else {
			log.Println(ldebugp, "kcp->cli:", rn)
		}
		appevt.Trigger("reqbytes", rn, len(buf)+25)
	}
}

func (this *KcpTransport) processKcpReadyRead_dep(ch *Channel) {
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
		// ch = this.ch
		log.Println(ldebugp, "processing channel data:", len(pkt.data), gopp.StrSuf(pkt.data, 52))
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
		// kcp定时poll转换为触发poll
		this.kcpPollChan <- KcpPollEvent{}
	}
}

const (
	kcp_mode_default = 0 // faketcp
	kcp_mode_noctrl  = 1
	kcp_mode_fast    = 2
)

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

package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"reflect"
	// "strings"
	"strconv"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/crypto/ecies"
	"github.com/ethereum/go-ethereum/logger/glog"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/discover"
	"github.com/ethereum/go-ethereum/p2p/nat"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/whisper/whisperv2"
)

// TODO only port mode
type EthereumTransport struct {
	TransportBase
	shh *whisperv2.Whisper
	srv *p2p.Server
	// udpSrv            net.PacketConn
	// readyReadDataChan chan UdpReadyReadEvent
	peerIP   string
	localIP  string
	port     int
	peerAddr net.Addr
}

func NewEthereumTransport(server bool) *EthereumTransport {
	tp := newEthereumTransport()
	obip := getOutboundIp()
	if !isReservedIpStr(obip) {
		tp.enable = true
	}

	tp.lossy = false
	tp.server = server
	tp.localIP = obip

	tp.init()
	go tp.serve()
	return tp
}

func newEthereumTransport() *EthereumTransport {
	tp := &EthereumTransport{}
	tp.lossy = false
	return tp
}

func (this *EthereumTransport) init() bool {
	this.readyReadNoticeChan = make(chan CommonEvent, mpcsz)
	// this.readyReadDataChan = make(chan UdpReadyReadEvent, mpcsz)
	this.readyReadDataChanType = reflect.TypeOf(this.readyReadNoticeChan)

	if this.server {
		return this.initServer()
	} else {
		return this.initServer()
	}
}

func (this *EthereumTransport) localVirtAddr() string {
	return this.localVirtAddr_
}

func (this *EthereumTransport) initServer() bool {
	log.Println(ldebugp)
	this.shh, this.srv = this.newEthereumChatServer()
	if true {
		return true
	}

	for i := 0; i < 256; i++ {
		addr := fmt.Sprintf(":%d", 18588+i)
		udpSrv, err := net.ListenPacket("udp", addr)
		if err != nil {
			// log.Println(lerrorp, err, udpSrv)
		} else {
			log.Println(linfop, "Listen UDP:", udpSrv.LocalAddr().String())
			// this.udpSrv = udpSrv
			this.port = 18588 + i
			this.localVirtAddr_ = fmt.Sprintf("%s:%d", this.localIP, this.port)
			break
		}
	}
	/*
		if this.udpSrv == nil {
			log.Fatalln("can not listen UDP port: (%d, %d)", 18588, 18588+256)
		}
	*/
	return true
}

func (this *EthereumTransport) initClient() bool {
	// connect to server?
	return true
}

func (this *EthereumTransport) serve() {
	log.Println(ldebugp, this.localIP, this.peerIP)
	if this.server {
		this.serveServer()
	} else {
		this.serveServer()
	}
}

func (this *EthereumTransport) serveServer() {
	/*
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
				this.peerAddr = addr
				evt := UdpReadyReadEvent{addr, buf[0:rdn], rdn}
				this.readyReadNoticeChan <- CommonEvent{reflect.TypeOf(evt), reflect.ValueOf(evt)}
				log.Println(ldebugp, "net->udp:", rdn, addr.String())
			}
		}
	*/
}

func (this *EthereumTransport) getEventData(evt CommonEvent) ([]byte, int, interface{}) {
	buf := evt.v.Interface().([]byte)
	return buf, len(buf), nil
}

func (this *EthereumTransport) serveClient() {

}

func (this *EthereumTransport) getReadyReadChanType() reflect.Type {
	return this.readyReadDataChanType
}

func (this *EthereumTransport) getReadyReadChan() <-chan CommonEvent {
	return this.readyReadNoticeChan
}

func (this *EthereumTransport) getConn() interface{} {
	return nil
}

func (this *EthereumTransport) sendDataBytes(buf []byte, size int, uaddr string) int {
	if this.server {
		return this.sendDataServer(buf, size, uaddr)
	} else {
		return this.sendDataClient(buf, size, uaddr)
	}
}
func (this *EthereumTransport) sendData(buf string, toaddr string) error {
	if this.server {
		this.sendDataServer([]byte(buf), len(buf), toaddr)
	} else {
		this.sendDataClient([]byte(buf), len(buf), toaddr)
	}
	return nil
}

func (this *EthereumTransport) sendDataServer(buf []byte, size int, uaddr string) int {
	// unused(uaddr) // we don't use passed uaddr
	msg := whisperv2.NewMessage(buf)

	keyfile := ""
	if this.server {
		keyfile = fmt.Sprintf("%d.ethkey.txt", 30303)
	} else {
		keyfile = fmt.Sprintf("%d.ethkey.txt", 30308)
	}
	// log.Println("loading...", keyfile)
	PrivateKey, err := crypto.LoadECDSA(keyfile)
	// msg.To = &PrivateKey.PublicKey
	// log.Println(msg.To)
	if err != nil {
		log.Println(err)
	}
	// log.Println(crypto.PubkeyToAddress(PrivateKey.PublicKey).Hex())

	if false {
		topics := whisperv2.NewTopicsFromStrings("topic01", "topic02", "topic03")
		envel := whisperv2.NewEnvelope(50*time.Second, topics, msg)
		log.Println(envel)
	}

	// m2
	eopts := whisperv2.Options{}
	// eopts.Topics = topics
	eopts.To = &PrivateKey.PublicKey
	if this.server {
		eopts.TTL = 60 * time.Second
	} else {
		eopts.TTL = 61 * time.Second
	}
	eopts.From = this.srv.PrivateKey
	envel, err := msg.Wrap(whisperv2.DefaultPoW, eopts)
	if err != nil {
		log.Println(err)
	}

	err = this.shh.Send(envel)
	if err != nil {
		log.Println(err)
	}
	log.Println("send data:", len(buf), len(msg.Payload))

	/*
		if this.peerAddr == nil {
			log.Println(lwarningp, "still not got the peerAddr")
			return -1
		}

				wrn, err := this.udpSrv.WriteTo(buf[:size], this.peerAddr)
				if err != nil {
					log.Println(lerrorp, err, wrn)
				} else {
					log.Println(ldebugp, "udp->net:", wrn)
				}
			return wrn
	*/
	return 0
}

func (this *EthereumTransport) sendDataClient(buf []byte, size int, toaddr string) int {
	return this.sendDataServer(buf, size, toaddr)
	/*
		uaddr, err := net.ResolveUDPAddr("udp", toaddr)
		if err != nil {
			log.Println(lerrorp, err, uaddr)
		}


			// replace with? this.sendDataServer(buf, size, uaddr)
			wrn, err := this.udpSrv.WriteTo(buf[:size], uaddr)
			if err != nil {
				log.Println(lerrorp, err, wrn)
			}
			return wrn
	*/
	return 0
}

////
var eport int = 30303 + 5

func init() {
	glog.SetToStderr(true)
	// glog.SetV(8)
	// flag.Var(glog.GetVerbosity(), "verbosity", "log verbosity (0-9)")
	*glog.GetVerbosity() = 4
	flag.IntVar(&eport, "eport", eport, "ethereum net port")
}

func (this *EthereumTransport) newEthereumChatServer() (*whisperv2.Whisper, *p2p.Server) {
	var err error

	// whisper
	whs := whisperv2.New()
	filterID := whs.Watch(whisperv2.Filter{Fn: this.shh_message_handler})
	// err = whs.Start(&srv)
	// if err != nil {
	// 		log.Println(err)
	//	}
	log.Println(whs.Protocols(), filterID)
	// msg := whisperv2.NewMessage([]byte{})

	// server node
	cfg := p2p.Config{}
	cfg.Name = "ethoy"
	cfg.Discovery = true
	cfg.MaxPeers = 8
	cfg.MaxPendingPeers = 16
	cfg.ListenAddr = ":30305"
	cfg.ListenAddr = fmt.Sprintf(":%d", eport)
	// cfg.NodeDatabase = "./vardb/"
	cfg.Protocols = whs.Protocols()
	cfg.NAT = nat.Any()

	for _, nurl := range params.MainnetBootnodes {
		cfg.BootstrapNodes = append(cfg.BootstrapNodes, discover.MustParseNode(nurl))
		// srv.AddPeer(discover.MustParseNode(nurl))
	}
	log.Println(cfg.BootstrapNodes)

	keyfile := fmt.Sprintf("%d.ethkey.txt", eport)
	cfg.PrivateKey, err = crypto.LoadECDSA(keyfile)
	if err != nil {
		log.Println(err)
		cfg.PrivateKey, err = crypto.GenerateKey()
		err = crypto.SaveECDSA(keyfile, cfg.PrivateKey)

	}

	srv := p2p.Server{Config: cfg}
	err = srv.Start()
	log.Println(err, srv.Self().String(), crypto.PubkeyToAddress(cfg.PrivateKey.PublicKey).Hex())
	// pubkey hex: server:20308: 0x6a574c29241690b4841cc6ed02f96bf1eff6a61d
	// pubkey hex: client:30303: 0x4e39b37c40dd037ebd7dbf97ac14bb7adac84911

	err = whs.Start(&srv)
	if err != nil {
		log.Println(err)
	}

	shhsrv = &srv
	return whs, &srv
}

var shhsrv *p2p.Server

func (this *EthereumTransport) shh_message_handler(msg *whisperv2.Message) {
	/*
		log.Printf("%+v\n", msg)
		log.Println(crypto.PubkeyToAddress(*msg.Recover()).Hex(),
			crypto.PubkeyToAddress(this.srv.PrivateKey.PublicKey).Hex())
		if msg.To != nil && *msg.To == this.srv.PrivateKey.PublicKey {
			log.Println(ldebugp, "myself msg")
			return
		}
		log.Println(msg.TTL, len(msg.Payload), string(msg.Payload), "/", msg.Hash.Str())
		decbuf, err := crypto.Decrypt(this.srv.PrivateKey, msg.Payload)
		if err != nil {
			log.Println(err)
		} else {
			log.Println("decrypted data:", len(decbuf))
		}
	*/
	log.Println(msg.TTL, len(msg.Payload))
	//this.readyReadNoticeChan <- newCommonEvent(msg.Payload)
	isSelfMsg := func() bool {
		if this.server {
			if msg.TTL == 60*time.Second {
				return true
			}
		} else {
			if msg.TTL == 61*time.Second {
				return true
			}
		}
		return false
	}
	isPrintable := func(s string) bool {
		for _, c := range s {
			if !strconv.IsPrint(rune(c)) {
				// log.Println(i, c, s[i:i+1], "/")
				return false
			}
		}
		return true
	}

	if isSelfMsg() {
		log.Println(ldebugp, "myself msg")
	} else {
		// log.Println(msg.TTL, len(msg.Payload), string(msg.Payload), "/", msg.Hash.Str())
		decbuf, err := crypto.Decrypt(this.srv.PrivateKey, msg.Payload)
		if err != nil {
			if isPrintable(string(msg.Payload)) {
				log.Println(err, len(msg.Payload), string(msg.Payload))
			} else {
				log.Println(err, len(msg.Payload))
			}

			switch err {
			case ecies.ErrInvalidMessage:
			case ecies.ErrInvalidPublicKey:
			default:
			}
		} else {
			log.Println("decrypted data:", len(decbuf), decbuf[0:6], string(decbuf[0:6]), string(decbuf))
			// log.Println(strings.Index(string(decbuf), "{"))
			// why pos=24? it's kcp's header
			this.readyReadNoticeChan <- newCommonEvent(decbuf)
		}
	}

}

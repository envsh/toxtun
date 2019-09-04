package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/binary"
	"fmt"
	"gopp"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"

	quic "github.com/lucas-clemente/quic-go"
	deadlock "github.com/sasha-s/go-deadlock"
	"github.com/xtaci/smux"

	rudp "mkuse/rudp2"
)

type MuxSession interface {
	OpenStream(syndat string) (MuxStream, error)
	AcceptStream() (MuxStream, error)
	IsClosed() bool
	Close() error
	SessID() uint32
}

type MuxStream interface {
	Syndat() string
	StreamID() uint32
	net.Conn
}

type MuxListener interface {
	// Close the server, sending CONNECTION_CLOSE frames to each peer.
	Close() error
	// Addr returns the local network addr that the server is listening on.
	// Addr() net.Addr
	// Accept returns new sessions. It should be called in a loop.
	Accept() (MuxSession, error)
}

/////
type QuicMux struct {
	conn  net.PacketConn
	lsner quic.Listener
}

type QuicSession struct {
	sess   quic.Session
	closed uint32
}

type QuicStream struct {
	stm    quic.Stream
	syndat string
}

func (this *QuicSession) OpenStream(syndat string) (MuxStream, error) {
	stm, err := this.sess.OpenStreamSync()
	this.chksetclosed(err)
	if err != nil {
		return nil, err
	}

	return &QuicStream{stm, syndat}, nil
}

func (this *QuicSession) AcceptStream() (MuxStream, error) {
	stm, err := this.sess.AcceptStream()
	this.chksetclosed(err)
	if err != nil {
		return nil, err
	}

	syndat := this.sess.RemoteAddr().String()
	return &QuicStream{stm, syndat}, nil
}
func (this *QuicSession) chksetclosed(err error) {
	if err != nil {
		atomic.CompareAndSwapUint32(&this.closed, 0, 1)
	}
}

func (this *QuicSession) Close() error {
	atomic.StoreUint32(&this.closed, 1)
	return this.sess.Close()
}

func (this *QuicSession) IsClosed() bool {
	return atomic.LoadUint32(&this.closed) == 1
}
func (this *QuicSession) SessID() uint32 {
	return 123
}

func (this *QuicStream) Close() error {
	return this.stm.Close()
}
func (this *QuicStream) Read(buf []byte) (int, error) {
	return this.stm.Read(buf)
}
func (this *QuicStream) Write(buf []byte) (int, error) {
	return this.stm.Write(buf)
}
func (this *QuicStream) Syndat() string {
	return this.syndat
}

func (this *QuicStream) StreamID() uint32                 { return uint32(this.stm.StreamID().StreamNum()) }
func (this *QuicStream) LocalAddr() net.Addr              { return nil }
func (this *QuicStream) RemoteAddr() net.Addr             { return nil }
func (this *QuicStream) SetDeadline(time.Time) error      { return nil }
func (this *QuicStream) SetReadDeadline(time.Time) error  { return nil }
func (this *QuicStream) SetWriteDeadline(time.Time) error { return nil }

func QuicListen(conn net.PacketConn) (MuxListener, error) {
	tlscfg := gopp.GenerateTLSConfig()
	quicfg := &quic.Config{}
	quicfg.KeepAlive = true

	lsner, err := quic.Listen(conn, tlscfg, quicfg)
	if err != nil {
		return nil, err
	}

	return &QuicMux{conn, lsner}, nil
}

func (this *QuicMux) Accept() (MuxSession, error) {
	sess, err := this.lsner.Accept()
	if err != nil {
		return nil, err
	}

	return &QuicSession{sess, 0}, nil
}
func (this *QuicMux) Close() error {
	return this.lsner.Close()
}

func QuicDial(conn net.PacketConn, remoteAddr net.Addr, host string) (MuxSession, error) {
	tlscfg := &tls.Config{}
	tlscfg.InsecureSkipVerify = true
	quicfg := &quic.Config{}
	quicfg.KeepAlive = true

	sess, err := quic.Dial(conn, remoteAddr, host, tlscfg, quicfg)
	if err != nil {
		return nil, err
	}

	return &QuicSession{sess, 0}, nil
}

/////
type RudpMux struct {
	conn net.PacketConn

	newinconnC chan uint32
	dialackchs sync.Map // conv => chan uint32
	sesses     sync.Map // accepted/dialed sessions conv => *RudpSession
}
type RudpSession struct {
	raddr    net.Addr
	conv     uint32
	accepted bool
	sess     *smux.Session
	rudp_    *rudp.RUDP
}
type RudpStream struct {
	stm *smux.Stream
}

func NewRudpSession(conv uint32, raddr net.Addr, accepted bool,
	writeoutfn func(*rudp.PfxByteArray, bool) error) *RudpSession {
	this := &RudpSession{}
	this.conv = conv
	this.accepted = accepted
	this.rudp_ = rudp.NewRUDP(conv, writeoutfn)

	cfg := &smux.Config{}
	cfg = &*smux.DefaultConfig()
	cfg.KeepAliveInterval *= 3
	cfg.KeepAliveTimeout *= 3 // 30

	var sess *smux.Session
	var err error
	if accepted {
		sess, err = smux.Server(this, cfg)
	} else {
		sess, err = smux.Client(this, cfg)
	}
	gopp.ErrPrint(err)
	this.sess = sess

	return this
}

func (this *RudpSession) input(buf []byte) (int, error) {
	this.rudp_.Input(buf)
	return len(buf), nil
}

func (this *RudpSession) Read(buf []byte) (int, error) {
	return this.rudp_.Read(buf)
}

func (this *RudpSession) Write(buf []byte) (int, error) {
	return this.rudp_.Write(buf)
}

func (this *RudpSession) Close() error {
	err := this.sess.Close()
	err1 := this.rudp_.Close()
	gopp.ErrPrint(err1)
	return err
}
func (this *RudpSession) Closed() bool {
	return this.sess.IsClosed()
}

func (this *RudpSession) SessID() uint32 { return this.conv }

func (this *RudpSession) OpenStream(syndat string) (MuxStream, error) {
	stm, err := this.sess.OpenStream(syndat)
	if err != nil {
		return nil, err
	}
	return &RudpStream{stm}, nil
}

func (this *RudpSession) AcceptStream() (MuxStream, error) {
	stm, err := this.sess.AcceptStream()
	if err != nil {
		return nil, err
	}
	return &RudpStream{stm}, nil
}
func (this *RudpSession) IsClosed() bool {
	return this.sess.IsClosed()
}

func (this *RudpStream) Close() error {
	return this.stm.Close()
}
func (this *RudpStream) Read(buf []byte) (int, error) {
	return this.stm.Read(buf)
}
func (this *RudpStream) Write(buf []byte) (int, error) {
	return this.stm.Write(buf)
}
func (this *RudpStream) Syndat() string {
	return this.stm.Syndat()
}

func (this *RudpStream) StreamID() uint32                 { return this.stm.ID() }
func (this *RudpStream) LocalAddr() net.Addr              { return nil }
func (this *RudpStream) RemoteAddr() net.Addr             { return nil }
func (this *RudpStream) SetDeadline(time.Time) error      { return nil }
func (this *RudpStream) SetReadDeadline(time.Time) error  { return nil }
func (this *RudpStream) SetWriteDeadline(time.Time) error { return nil }

/*
Listen listens for QUIC connections on a given net.PacketConn. A single PacketConn only be used for a single call to Listen. The PacketConn can be used for simultaneous calls to Dial. QUIC connection IDs are used for demultiplexing the different connections. The tls.Config must not be nil and must contain a certificate configuration. The quic.Config may be nil, in that case the default values will be used.
*/
func RudpListen(conn net.PacketConn) MuxListener {
	mx := rudplsnerman.createlsner(conn)
	return mx
}

func NewRudpMux(conn net.PacketConn) *RudpMux {
	this := &RudpMux{}
	this.conn = conn
	this.newinconnC = make(chan uint32, 1)
	return this
}

func (this *RudpMux) Accept() (MuxSession, error) {
	conv, ok := <-this.newinconnC
	if !ok {
		return nil, fmt.Errorf("accept failed/closed")
	}
	if sessx, ok := this.sesses.Load(conv); ok {
		return sessx.(*RudpSession), nil
	}
	return nil, fmt.Errorf("Unknown error %v", conv)
}
func (this *RudpMux) Close() error {
	var sesses = map[interface{}]*RudpSession{}
	this.sesses.Range(func(key interface{}, value interface{}) bool {
		sesses[key] = value.(*RudpSession)
		return true
	})
	for key, sess := range sesses {
		err := sess.Close()
		gopp.ErrPrint(err)
		if err == nil {
			this.sesses.Delete(key)
		}
	}
	return nil
}

func (this *RudpMux) makewriteoutfn(raddr net.Addr) func(data *rudp.PfxByteArray, prior bool) error {
	return func(data *rudp.PfxByteArray, prior bool) error {
		_, err := this.conn.WriteTo(data.FullData(), raddr)
		return err
	}
}

func (this *RudpMux) dispatchProc() {
	conn := this.conn

	rbuf := make([]byte, 4096)
	for {
		rn, addr, err := conn.ReadFrom(rbuf)
		gopp.ErrPrint(err)
		if err != nil {
			break
		}
		_ = addr

		tbuf := gopp.BytesDup(rbuf[:rn])
		var conv uint32

		// check if new incoming conn handshake
		if bytes.HasPrefix(tbuf, RUDPMUXCONNSYN) {
			conv = binary.LittleEndian.Uint32(tbuf[len(RUDPMUXCONNSYN):])
			log.Println("new rudp mux conn syn", conv)
			sess := NewRudpSession(conv, addr, true, this.makewriteoutfn(addr))
			this.sesses.Store(conv, sess)
			this.newinconnC <- conv

			pkt := bytes.NewBuffer(nil)
			pkt.Write(RUDPMUXCONNACK)
			binary.Write(pkt, binary.LittleEndian, conv)
			_, err := conn.WriteTo(pkt.Bytes(), addr)
			gopp.ErrPrint(err, conv)
			if err != nil {
				log.Println("wtf, send mux conn ack failed", conv, addr.Network(), addr.String())
			}
			log.Println("sent mux conn ack", err)
			continue
		}

		// check if connack
		if bytes.HasPrefix(tbuf, RUDPMUXCONNACK) {
			conv = binary.LittleEndian.Uint32(tbuf[len(RUDPMUXCONNSYN):])
			log.Println("rudp mux conn ack", conv)
			sess := NewRudpSession(conv, addr, false, this.makewriteoutfn(addr))
			this.sesses.Store(conv, sess)
			if chx, ok := this.dialackchs.Load(conv); ok {
				ch := chx.(chan uint32)
				ch <- conv
			} else {
				log.Println("no one waiting", conv)
			}
			continue
		}

		// check if connrst
		if bytes.HasPrefix(tbuf, RUDPMUXCONNRST) {
			conv = binary.LittleEndian.Uint32(tbuf[len(RUDPMUXCONNSYN):])
			log.Println("rudp mux conn rst", conv)
			if sessx, ok := this.sesses.Load(conv); ok {
				sess := sessx.(*RudpSession)
				err := sess.Close()
				gopp.ErrPrint(err, sess.SessID(), conv)
			} else {
				log.Println("rudp mux conn rst not found", conv)
			}
			continue
		}

		// check others
		if bytes.HasPrefix(tbuf, RUDPMUXCONNSYN[:len(RUDPMUXCONNSYN)-3]) {
			log.Println("Unimpl cmd", string(tbuf[:len(RUDPMUXCONNSYN)]))
			continue
		}

		// dispatch to sess by conv
		conv = binary.LittleEndian.Uint32(tbuf)
		if sessx, ok := this.sesses.Load(conv); ok {
			sess := sessx.(*RudpSession)
			sess.input(tbuf)
		} else {
			log.Println("conv not found", conv) // is possible reply a close???
			pkt := bytes.NewBuffer(nil)
			pkt.Write(RUDPMUXCONNRST)
			binary.Write(pkt, binary.LittleEndian, conv)
			_, err := conn.WriteTo(pkt.Bytes(), addr)
			gopp.ErrPrint(err, conv)
			if err != nil {
				log.Println("wtf, send mux conn rst failed", conv, addr.Network(), addr.String())
			}
			log.Println("sent mux conn rst", err)
		}
	}
}

var RUDPMUXCONNSYN = []byte("RUDPMUXCONNSYN")
var RUDPMUXCONNACK = []byte("RUDPMUXCONNACK")
var RUDPMUXCONNRST = []byte("RUDPMUXCONNRST")

// 1 pktconn : n sess : m stm

func (this *RudpMux) dialsync(ctx context.Context, addr net.Addr) (MuxSession, error) {
	conn := this.conn
	if conn == nil {
		log.Println("wtf conn nil???")
	}
	conv := rudplsnerman.nextconv()
	ackch := make(chan uint32, 0)
	this.dialackchs.Store(conv, ackch)
	defer this.dialackchs.Delete(conv)

	pkt := bytes.NewBuffer(nil)
	pkt.Write(RUDPMUXCONNSYN)
	binary.Write(pkt, binary.LittleEndian, conv)
	_, err := conn.WriteTo(pkt.Bytes(), addr)
	if err != nil {
		return nil, err
	}

	select {
	case conv := <-ackch:
		if sessx, ok := this.sesses.Load(conv); ok {
			return sessx.(*RudpSession), nil
		} else {
			return nil, fmt.Errorf("Unknown error %v", conv)
		}
	case <-time.After(10 * time.Second):
		log.Println("dial timeout", conv)
		return nil, fmt.Errorf("dial timeout %v", conv)
	}
}

func RudpDialSync(conn net.PacketConn, raddr net.Addr) (MuxSession, error) {
	mx := rudplsnerman.createlsner(conn)
	return mx.dialsync(context.Background(), raddr)
}

//
type rudplsnermanager struct {
	lsners map[net.PacketConn]*RudpMux
	mu     deadlock.RWMutex
	conv   uint32
}

func newrudplsnermanager() *rudplsnermanager {
	this := &rudplsnermanager{}
	this.lsners = map[net.PacketConn]*RudpMux{}
	this.conv = 1234567890
	return this
}

func (this *rudplsnermanager) nextconv() uint32 { return atomic.AddUint32(&this.conv, 1) }

func (this *rudplsnermanager) createlsner(conn net.PacketConn) *RudpMux {
	this.mu.Lock()
	defer this.mu.Unlock()

	if mx, ok := this.lsners[conn]; !ok {
		mx = NewRudpMux(conn)
		go this.servemux(mx)
		this.lsners[conn] = mx
	}
	return this.lsners[conn]
}

func (this *rudplsnermanager) servemux(mx *RudpMux) {
	mx.dispatchProc()
	// delete from lsners???
}

var rudplsnerman = newrudplsnermanager()

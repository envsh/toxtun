package main

import (
	"log"
	"net"
	"sync"
	"time"
)

type UDPServer struct {
	pc        net.PacketConn
	clients   map[string]net.Conn // *UDPConn2
	clientsmu sync.Mutex
}

func ListenUDP(address string) (net.Listener, error) {
	lsner := &UDPServer{}
	pc, err := net.ListenPacket("udp", address)
	if err != nil {
		return nil, err
	}
	lsner.pc = pc
	lsner.clients = make(map[string]net.Conn)
	return lsner, nil
}

// should block
func (this *UDPServer) Accept() (net.Conn, error) {
	// zrbuf := make([]byte, 0)
	for {
		kbuf := make([]byte, 1024) // TODO what size
		rn, addr, err := this.pc.ReadFrom(kbuf)
		log.Println(rn, addr, err, string(kbuf[:rn]))
		if err != nil {
			log.Println(err)
			return nil, err
		}
		hasnew := false
		var newc net.Conn
		this.clientsmu.Lock()
		if newc2, ok := this.clients[addr.String()]; ok {
			newc = newc2
		} else {
			// think as new connection
			newc = NewUDPConn2(this.pc, addr)
			this.clients[addr.String()] = newc
			hasnew = true
		}
		this.clientsmu.Unlock()

		// check chan close and don't send data
		func() {
			defer func() {
				if err := recover(); err != nil {
					log.Println(err, hasnew)
					if err.(error).Error() == "send on closed channel" {
						this.clientsmu.Lock()
						delete(this.clients, addr.String())
						newc = NewUDPConn2(this.pc, addr)
						this.clients[addr.String()] = newc
						hasnew = true
						this.clientsmu.Unlock()
					}
				}
			}()
			newc.(*UDPConn2).InChan <- kbuf[:rn]
		}()

		if !hasnew {
			// time.Sleep(1 * time.Microsecond)
		} else {
			return newc, nil
		}
	}
}

func (this *UDPServer) Close() error   { return this.pc.Close() }
func (this *UDPServer) Addr() net.Addr { return this.pc.LocalAddr() }

/////
type dummyConn struct{}

func (this *dummyConn) Close() error                       { return nil }
func (this *dummyConn) SetDeadline(t time.Time) error      { return nil }
func (this *dummyConn) SetReadDeadline(t time.Time) error  { return nil }
func (this *dummyConn) SetWriteDeadline(t time.Time) error { return nil }

/*
	Read(b []byte) (n int, err error)
	Write(b []byte) (n int, err error)
	Close() error
	LocalAddr() Addr
	RemoteAddr() Addr
	SetDeadline(t time.Time) error
	SetReadDeadline(t time.Time) error
	SetWriteDeadline(t time.Time) error

*/
type UDPConn2 struct {
	// *dummyConn
	Conn   net.PacketConn
	Raddr  *net.UDPAddr
	InChan chan []byte
}

func NewUDPConn2(c net.PacketConn, raddr net.Addr) net.Conn {
	this := &UDPConn2{}
	// this.dummyConn = &dummyConn{}
	this.Conn = c
	this.Raddr = raddr.(*net.UDPAddr)
	this.InChan = make(chan []byte, 1)
	return this
}

func (this *UDPConn2) Read(b []byte) (n int, err error) {
	nb := <-this.InChan
	rn := copy(b, nb)
	if rn < len(nb) {
		panic("buffer too small")
	}
	return rn, nil
}
func (this *UDPConn2) Write(b []byte) (n int, err error) {
	return this.Conn.WriteTo(b, this.Raddr)
}
func (this *UDPConn2) Close() (err error) {
	log.Println(123)
	close(this.InChan)
	return
}
func (this *UDPConn2) LocalAddr() net.Addr                { return this.Conn.LocalAddr() }
func (this *UDPConn2) RemoteAddr() net.Addr               { return this.Raddr }
func (this *UDPConn2) SetDeadline(t time.Time) error      { return nil }
func (this *UDPConn2) SetReadDeadline(t time.Time) error  { return nil }
func (this *UDPConn2) SetWriteDeadline(t time.Time) error { return nil }

/////
type UDPConn struct {
	net.PacketConn
	Raddr *net.UDPAddr
}

func (this *UDPConn) Close() error                { return nil }
func (this *UDPConn) RemoteAddr() net.Addr        { return this.Raddr }
func (this *UDPConn) Write(b []byte) (int, error) { return this.PacketConn.WriteTo(b, this.Raddr) }
func (this *UDPConn) Read(b []byte) (int, error) {
	n, addr, err := this.ReadFrom(b)
	if addr.String() != this.Raddr.String() {
	}
	return n, err
}

func NewUDPConnOnServer(srv net.PacketConn, raddr net.Addr) net.Conn {
	c := &UDPConn{PacketConn: srv, Raddr: raddr.(*net.UDPAddr)}
	return c
}

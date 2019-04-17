package main

import (
	"gopp"
	rudp3 "mkuse/rudp3"
	"net"
	"sync/atomic"
)

type Rudp3Listener struct {
	conn     net.PacketConn
	sess     *Rudp3Session
	accepthc chan bool
}

func NewRudp3Listener(conn net.PacketConn) *Rudp3Listener {
	this := &Rudp3Listener{}
	this.conn = conn
	sess := rudp3.NewSession(conn, "tokip-")
	this.sess = NewRudp3Session(sess)
	this.accepthc = make(chan bool, 1)
	this.accepthc <- true
	return this
}

// only return once
func (this *Rudp3Listener) Accept() (MuxSession, error) {
	stop := false
	for !stop {
		select {
		case <-this.accepthc:
			return this.sess, nil
		}
	}
	return nil, nil
}

func (this *Rudp3Listener) Close() error { return nil }

var _ MuxStream = &rudp3.Stream{}
var _ MuxListener = &Rudp3Listener{}

type Rudp3Session struct {
	*rudp3.Session
	sessid uint32
}

var rudp3sessid uint32 = 99

func NewRudp3Session(sess *rudp3.Session) *Rudp3Session {
	this := &Rudp3Session{}
	this.Session = sess
	this.sessid = atomic.AddUint32(&rudp3sessid, 1)
	return this
}

func (this *Rudp3Session) AcceptStream() (MuxStream, error) {
	s := this.Session
	stm, err := s.Accept()
	gopp.ErrPrint(err)
	if err != nil {
		return nil, err
	}
	return stm, nil
}

func (this *Rudp3Session) OpenStream(syndat string) (MuxStream, error) {
	s := this.Session
	stm, err := s.OpenStream(syndat)
	gopp.ErrPrint(err)
	if err != nil {
		return nil, err
	}
	return stm, nil
}

func (this *Rudp3Session) SessID() uint32 { return this.sessid }

func (this *Rudp3Session) Close() error   { return nil }
func (this *Rudp3Session) IsClosed() bool { return false }

package main

import tox "github.com/TokTok/go-toxcore-c"

/// factory
type MuxManager struct {
}

// by pubkey, stream id
func (this *MuxManager) DispatchIncoming() {

}

///
type ToxMuxStream struct {
}

func (this *ToxMuxStream) Read() {

}

func (this *ToxMuxStream) Write() {

}

///
type ToxMuxBase struct {
	t *tox.Tox
}

///
type ToxMuxClient struct {
	ToxMuxBase

	srv_pubkey string
	conn_state string //
}

func NewToxMuxClient(t *tox.Tox, srv_pubkey string) *ToxMuxClient {
	this := &ToxMuxClient{}
	this.t = t
	this.srv_pubkey = srv_pubkey
	return this
}

func (this *ToxMuxClient) OpenStream() *ToxMuxStream {
	return nil
}

///
type ToxMuxServer struct {
	ToxMuxBase
}

func NewToxMuxServer(t *tox.Tox) *ToxMuxServer {
	this := &ToxMuxServer{}
	this.t = t
	return this
}

func (this *ToxMuxServer) AcceptStream() *ToxMuxStream {
	return nil
}

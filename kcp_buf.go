package main

import (
	"sync"
	"time"
)

var (
	xmitBuf sync.Pool
)

const (
	defaultWndSize           = 128 // default window size, in packet
	nonceSize                = 16  // magic number
	crcSize                  = 4   // 4bytes packet checksum
	cryptHeaderSize          = nonceSize + crcSize
	mtuLimit                 = 2048
	rxQueueLimit             = 8192
	rxFECMulti               = 3 // FEC keeps rxFECMulti* (dataShard+parityShard) ordered packets in memory
	defaultKeepAliveInterval = 10
)

func init() {
	xmitBuf.New = func() interface{} {
		return make([]byte, mtuLimit)
	}
}

func currentMs() uint32 {
	return uint32(time.Now().UnixNano() / int64(time.Millisecond))
}

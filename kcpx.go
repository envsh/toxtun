package main

import (
	"encoding/binary"
)

func kcp_extract_data(data []byte) (ret []byte, rebuf []byte) {
	conv := binary.LittleEndian.Uint32(data)
	kcp := NewKCP(conv, func(buf []byte, size int, extra interface{}) { rebuf = buf[:size] }, nil)
	kcp.NoDelay(1, 1, 1, 1)
	n := kcp.Input(data)
	kcp.Update(uint32(iclock()))

	ret = make([]byte, kcp.PeekSize())
	p := kcp.Recv(ret)
	if false {
		debug.Println(n, p)
	}
	return
}

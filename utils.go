package main

import (
	"time"
)

func iclock() uint32 {
	return uint32((time.Now().UnixNano() / 1000000) & 0xffffffff)
}

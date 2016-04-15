package main

import (
	"time"
)

func iclock() int32 {
	return int32((time.Now().UnixNano() / 1000000) & 0xffffffff)
}

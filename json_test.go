package main

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"
)

func TestJson0(t *testing.T) {
	type ColorGroup struct {
		ByteSlice  []byte
		SingleByte byte
		IntSlice   []int
	}
	group := ColorGroup{
		ByteSlice:  []byte{0, 0, 0, 1, 2, 3},
		SingleByte: 10,
		IntSlice:   []int{0, 0, 0, 1, 2, 3},
	}
	b, err := json.Marshal(group)
	if err != nil {
		fmt.Println("error:", err)
	}
	os.Stdout.Write(b)
}

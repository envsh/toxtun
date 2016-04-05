package main

import (
	"flag"
	"log"
)

const (
	tunconv = uint32(0xaabbccdd)
	tunmtu  = 1000
	rdbufsz = 8192
)

var ()

func init() {
}

func main() {
	flag.Parse()
	argv := flag.Args()
	argc := len(argv)

	mode := ""
	if argc > 0 {
		mode = argv[argc-1]
	}

	switch mode {
	case "client":
		tc := NewTunnelc()
		tc.serve()
	case "server":
		td := NewTunneld()
		td.serve()
	default:
		log.Println("Invalid mode")
		flag.PrintDefaults()
	}
}

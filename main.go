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

var (
	kcp_mode = "default" // fast
)

func init() {
	flag.StringVar(&kcp_mode, "kcp-mode", "default", "default|fast")
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

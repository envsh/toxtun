package main

import (
	"flag"
	"log"
	"os"
)

const (
	tunconv = uint32(0xaabbccdd)
	tunmtu  = 1000
	rdbufsz = 8192
)

var (
	// options
	kcp_mode    string // = "default" // fast
	config_file string // = "toxtun_whtun.ini"
	config      *TunnelConfig
)

func init() {
	flag.StringVar(&kcp_mode, "kcp-mode", "default", "default|fast")
	if !(kcp_mode == "default" || kcp_mode == "fast") {
		kcp_mode = "default"
	}

	flag.StringVar(&config_file, "config", "", "config file .ini")
}

func main() {
	flag.Parse()
	if len(config_file) > 0 {
		config = NewTunnelConfig(config_file)
		info.Println(config)
	}

	argv := flag.Args()
	argc := len(argv)

	mode := ""
	if argc > 0 {
		mode = argv[argc-1]
	}

	go NewStatServer().serve()
	appevt.Trigger("appmode", mode)

	switch mode {
	case "client":
		if config == nil {
			flag.PrintDefaults()
			os.Exit(-1)
		}
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

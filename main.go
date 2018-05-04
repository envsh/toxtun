package main

import (
	"flag"
	"log"
	"os"
)

const (
	tunconv = uint32(0xaabbccdd)
	tunmtu  = 1300 // mtu+IKCP_OVERHEAD<tox.MAX_MESSAGE_LENGTH
	rdbufsz = 8192
)

var (
	// options
	inst_mode   string // = "server" | "client"
	kcp_mode    string // = "default" // fast
	config_file string // = "toxtun_whtun.ini"
	config      *TunnelConfig
)

func init() {
	flag.StringVar(&kcp_mode, "kcp-mode", "fast", "default|normal|fast|fast2|fast3")
	if !(kcp_mode == "default" || kcp_mode == "fast") {
		kcp_mode = "fast"
	}

	flag.StringVar(&config_file, "config", "", "config file .ini")
}

func main() {
	printBuildInfo(true)
	flag.Parse()
	if len(config_file) > 0 {
		config = NewTunnelConfig(config_file)
		info.Println(config)
	}

	set_speed_mode(kcp_mode)
	argv := flag.Args()
	argc := len(argv)
	if argc > 0 {
		inst_mode = argv[argc-1]
	}

	go NewStatServer().serve()
	appevt.Trigger("appmode", inst_mode)

	switch inst_mode {
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
		log.Println("Invalid mode:", inst_mode, ", server/client.")
		flag.PrintDefaults()
	}

}

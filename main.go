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
	inst_mode   string             // = "server" | "client"
	kcp_mode    string = "default" // = "default" // fast
	config_file string             // = "toxtun_whtun.ini"
	config      *TunnelConfig
)

func init() {
	flag.StringVar(&kcp_mode, "kcp-mode", "fast", "default|normal|fast|fast2|fast3")
	if !(kcp_mode == "default" || kcp_mode == "fast") {
		kcp_mode = "fast"
	}
	flag.IntVar(&smuse.interval, "interval", smuse.interval, "kcp check interval")
	flag.IntVar(&smuse.kcp_interval, "kcp-interval", smuse.kcp_interval, "kcp update interval")

	flag.StringVar(&config_file, "config", "", "config file .ini")

}

func main() {
	printBuildInfo(true)
	flag.Parse()
	if len(config_file) > 0 {
		config = NewTunnelConfig(config_file)
		info.Println(config)
	}

	argv := flag.Args()
	argc := len(argv)
	if argc > 0 {
		inst_mode = argv[argc-1]
	}

	set_bootstrap_group(inst_mode)
	// set_speed_mode(kcp_mode)
	info.Printf("Using packet format: %s, Using bs group:%s\n", pkt_use_fmt, tox_bs_group)

	go NewStatServer().serve()
	appevt.Trigger("appmode", inst_mode)

	SetupProfile()
	SetupGops()

	switch inst_mode {
	case "client":
		if config == nil {
			flag.PrintDefaults()
			os.Exit(-1)
		}
		tc := NewTunnelc()
		go tc.serve()
	case "server":
		td := NewTunneld()
		go td.serve()
	default:
		log.Println("Invalid mode:", inst_mode, ", server/client.")
		flag.PrintDefaults()
		return
	}

	SetupSignal(func() {
		StopProfile()
	})
}

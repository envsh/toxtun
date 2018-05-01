package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"runtime/pprof"
	"strings"
	"syscall"

	_ "net/http/pprof"

	"github.com/google/gops/agent"
	"github.com/kitech/colog"
)

const (
	tunconv  = uint32(0xaabbccdd)
	tunmtu   = 1000
	tunwndsz = tunmtu
	rdbufsz  = 8192
)

var (
	// options
	inst_mode   string // = "server" | "client"
	kcp_mode    string // fast
	config_file string // = "toxtun_whtun.ini"
	config      *TunnelConfig
	log_level   int = int(colog.LDebug)
	cpuprofile  string
)

// build info
var GitCommit, GitBranch, GitState, GitSummary, BuildDate, Version string

func getBuildInfo(full bool) string {
	trim := func(s string) string {
		if strings.HasPrefix(s, "GOVVV-") {
			return s[6:]
		}
		return s
	}
	commit := trim(GitCommit)
	branch := trim(GitBranch)
	// state := trim(GitState)
	summary := trim(GitSummary)
	date := trim(BuildDate)
	version := trim(Version)

	if full {
		return fmt.Sprintf("govvv: v%s branch:%s git:%s build:%s summary:%s, ",
			version, branch, commit, date, summary)
	}
	return fmt.Sprintf("govvv: v%s git:%s build:%s", version, commit, date)
}

const (
	ltracep   = "trace: "
	ldebugp   = "debug: "
	linfop    = "info: "
	lwarningp = "warning: "
	lerrorp   = "error: "
	lalertp   = "alert: "
)

func init() {
	log.SetFlags(log.Flags() | log.Lshortfile | log.Lmicroseconds)
	colog.Register()
	/*
		log.Println("debug: ")
		log.Println("info: ", 1)
		log.Println("warning: ", 1)
		log.Println("error: ", 1)
	*/

	// the same as kcptun
	flag.StringVar(&kcp_mode, "kcp-mode", "fast", "normal|fast|fast2|fast3")
	if !(kcp_mode == "fast" || kcp_mode == "noctrl" ||
		kcp_mode == "fast2" || kcp_mode == "fast3") {
		kcp_mode = "fast"
	}

	flag.StringVar(&config_file, "config", "", "config file .ini")
	flag.IntVar(&log_level, "log-level", int(colog.LDebug),
		fmt.Sprintf("%d - %d, ", colog.LTrace, colog.LAlert))
	flag.StringVar(&cpuprofile, "pprof", cpuprofile, "enable CPU pprof")
}

func main() {
	log.Println(getBuildInfo(true))
	flag.Parse()
	colog.SetDefaultLevel(colog.LDebug)
	colog.SetMinLevel(colog.LInfo)
	colog.SetMinLevel(colog.Level(log_level))
	if err := agent.Listen(agent.Options{}); err != nil {
		log.Fatal(err)
	}
	if len(config_file) > 0 {
		config = NewTunnelConfig(config_file)
		log.Println(linfop, config)
	}

	argv := flag.Args()
	argc := len(argv)

	if argc > 0 {
		inst_mode = argv[argc-1]
	}

	if cpuprofile != "" {
		f, err := os.Create(cpuprofile)
		if err != nil {
			log.Fatal(err)
		}
		pprof.StartCPUProfile(f)
		defer func() {
			log.Println("save profile")
			pprof.StopCPUProfile()
		}()
	}

	go NewStatServer().serve()
	appevt.Trigger("appmode", inst_mode)

	switch inst_mode {
	case "client":
		if config == nil {
			log.Println("error:", "need config file")
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
	}

	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)

	for {
		select {
		case s := <-c:
			switch s {
			case syscall.SIGINT:
				log.Println(linfop, "exiting...", s)
				// os.Exit(0) // will not run defer
				return // will run defer
			default:
				log.Println("unprocessed signal:", s)
			}
		}
	}
	// os.Exit 和 return main.main()的区别：
	// 前者不会调用defer，后者会。
}

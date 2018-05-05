package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"runtime/pprof"
	"syscall"

	"github.com/google/gops/agent"
)

const (
	ltracep   = "trace: "
	ldebugp   = "debug: "
	linfop    = "info: "
	lwarningp = "warning: "
	lerrorp   = "error: "
	lalertp   = "alert: "
)

var cpuprofile string

func init() {
	flag.StringVar(&cpuprofile, "pprof", cpuprofile, "enable CPU pprof, supply file name.")
}

func SetupProfile() {
	if cpuprofile != "" {
		f, err := os.Create(cpuprofile)
		if err != nil {
			log.Fatal(err)
		}
		pprof.StartCPUProfile(f)
	}
}

func StopProfile() {
	if cpuprofile != "" {
		log.Println("save profile", cpuprofile)
		pprof.StopCPUProfile()
	}
}

// should block
func SetupSignal(exitFunc func()) {
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)

	for {
		select {
		case s := <-c:
			switch s {
			case syscall.SIGINT:
				info.Println("exiting...", s)
				// os.Exit(0) // will not run defer
				if exitFunc != nil {
					exitFunc()
				}
				return // will run defer
			default:
				log.Println("unprocessed signal:", s)
			}
		}
	}
}

func SetupGops() {
	if err := agent.Listen(agent.Options{}); err != nil {
		log.Fatal(err)
	}
}

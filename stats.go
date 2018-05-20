package main

import (
	"log"
	"net/http"
	"sync"
	// "strings"
	"bytes"
	"fmt"
	"mime"
	"runtime"
	"time"

	"github.com/GianlucaGuarini/go-observable"
	"github.com/bitly/go-simplejson"
	// "github.com/davecgh/go-spew/spew"
)

type Stats struct {
	minGoRoutine int
	maxGoRoutine int
	minMem       uint64
	maxMem       uint64

	connCount      int
	connErrorCount int
	connOKCount    int
	connMinTime    time.Duration
	connMaxTime    time.Duration
	connAvgTime    time.Duration
	// 真正的成功的网络连接数
	connActiveCount int
	chanActiveCount int
	chanRealCount   int
	chan2RealCount  int
	closeReasons    map[string]int

	reqLen     uint64
	respLen    uint64
	reqNetLen  uint64
	respNetLen uint64

	appmode          string
	selfOnline       bool
	peerOnline       bool
	selfOfflineCount int
	peerOfflineCount int

	evtmu sync.Mutex
}

func NewStates() *Stats {
	crs := make(map[string]int, 0)
	sts := new(Stats)
	sts.closeReasons = crs
	return sts
}

var sts = NewStates()
var appevt = observable.New()

func init() {
	appevt.On("*", processEvent)
}
func processEvent(args ...interface{}) {
	sts.evtmu.Lock()
	defer sts.evtmu.Unlock()

	evt := args[0].(string)
	switch evt {
	case "newconn":
		sts.connCount += 1
	case "connerr":
		sts.connErrorCount += 1
	case "connok":
		sts.connOKCount += 1
	case "connact":
		sts.connActiveCount += args[1].(int)
	case "chanact":
		sts.chanActiveCount += args[1].(int)
		sts.chanRealCount = args[2].(int)
		sts.chan2RealCount = args[3].(int)
	case "closereason":
		sts.closeReasons[args[1].(string)] += 1
	case "reqbytes":
		sts.reqLen += uint64(args[1].(int))
		sts.reqNetLen += uint64(args[2].(int))
	case "respbytes":
		sts.respLen += uint64(args[1].(int))
		sts.respNetLen += uint64(args[2].(int))
	case "selfonline":
		sts.selfOnline = args[1].(bool)
	case "peeronline":
		sts.peerOnline = args[1].(bool)
	case "selfoffline":
		sts.selfOfflineCount += 1
	case "peeroffline":
		sts.peerOfflineCount += 1
	case "appmode":
		sts.appmode = args[1].(string)
	default:
	}
}

func updateStats() {
	sts.evtmu.Lock()
	defer sts.evtmu.Unlock()

	rnum := runtime.NumGoroutine()
	if sts.minGoRoutine == 0 {
		sts.minGoRoutine = rnum
	}
	if rnum < sts.minGoRoutine {
		sts.minGoRoutine = rnum
	}
	if rnum > sts.maxGoRoutine {
		sts.maxGoRoutine = rnum
	}

	msts := runtime.MemStats{}
	runtime.ReadMemStats(&msts)

	if sts.minMem == 0 {
		sts.minMem = msts.Alloc
	}
	if msts.Alloc < sts.minMem {
		sts.minMem = msts.Alloc
	}
	if msts.Alloc > sts.maxMem {
		sts.maxMem = msts.Alloc
	}

}

func indexHandler(w http.ResponseWriter, req *http.Request) {
	w.Header().Add("Content-Type", mime.TypeByExtension(".txt"))
	w.WriteHeader(200)

	sts.evtmu.Lock()
	defer sts.evtmu.Unlock()

	rnum := runtime.NumGoroutine()
	msts := runtime.MemStats{}
	runtime.ReadMemStats(&msts)

	wb := bytes.NewBufferString("")
	str := fmt.Sprintf("toxtun stats: %v\n", sts.appmode)

	str += fmt.Sprintf("Online Status: self: %v/%v, peer: %v/%v\n",
		sts.selfOnline, sts.selfOfflineCount, sts.peerOnline, sts.peerOfflineCount)
	str += fmt.Sprintf("Connections: total: %v, ok: %v, err: %v, active: %v, chans: %v, real1: %v, real2: %v\n",
		sts.connCount, sts.connOKCount, sts.connErrorCount, sts.connActiveCount, sts.chanActiveCount,
		sts.chanRealCount, sts.chan2RealCount)
	str += fmt.Sprintf("Close Reasons: %v\n", sts.closeReasons)

	dsrate := float64(0)
	if sts.reqLen > 0 {
		dsrate = float64(sts.respLen) / float64(sts.reqLen)
	}
	nsrate := float64(0)
	if sts.respLen > 0 {
		nsrate = float64(sts.respNetLen) / float64(sts.respLen)
	}
	str += fmt.Sprintf("DataStream: req: %v, resp: %v, r/s: 1/%.0f, reqnet: %v, respnet: %v, cost: %.2f\n",
		sts.reqLen, sts.respLen, dsrate, sts.reqNetLen, sts.respNetLen, nsrate)

	str += fmt.Sprintf("goroutines: cur: %v, max: %v, min: %v\n",
		rnum, sts.maxGoRoutine, sts.minGoRoutine)
	str += fmt.Sprintf("memory: cur: %v, max: %v, min: %v, total: %v\n",
		msts.Alloc, sts.maxMem, sts.minMem, msts.TotalAlloc)

	str += fmt.Sprintf("\nGo internals: %v\n", "")
	str += fmt.Sprintf("goroutines: %v\n", rnum)
	jso := simplejson.New()
	jso.Set("MemStats", msts)
	jbuf, err := jso.Encode()
	jso, err = simplejson.NewJson(jbuf)
	jso.Get("MemStats").Set("PauseNs", "--")
	jso.Get("MemStats").Set("PauseEnd", "--")
	jso.Get("MemStats").Set("BySize", "--")
	jstr, err := jso.Get("MemStats").EncodePretty()
	if err != nil {
	}
	str += "MemStats:\n"
	str += string(jstr)
	// str = str + spew.Sdump("mem stats: %v\n", msts)

	wb.WriteString(str)
	w.Write(wb.Bytes())
}

type StatServer struct {
}

func NewStatServer() *StatServer {
	return &StatServer{}
}

func (this *StatServer) serve() {
	go func() {
		for {
			updateStats()
			time.Sleep(1234 * time.Millisecond)
		}
	}()

	http.HandleFunc("/stats", indexHandler)

	for i := 9981; i < 9981+100; i++ {
		log.Println(linfop, "Try Listen stats port: ", i)
		err := http.ListenAndServe(fmt.Sprintf(":%d", i), nil)
		if err != nil {
			log.Println(lerrorp, err)
			continue
		} else {
			log.Println(linfop)
			break
		}
	}
}

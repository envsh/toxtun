package main

import (
	"net/http"
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

	connCount       int
	connErrorCount  int
	connOKCount     int
	connMinTime     time.Duration
	connMaxTime     time.Duration
	connAvgTime     time.Duration
	connActiveCount int
	chanActiveCount int

	recvLen uint64
	sendLen uint64

	appmode          string
	selfOnline       bool
	peerOnline       bool
	selfOfflineCount int
	peerOfflineCount int
}

var sts = Stats{}
var appevt = observable.New()

func init() {
	appevt.On("*", processEvent)
}
func processEvent(args ...interface{}) {
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
	case "recvbytes":
		sts.recvLen += uint64(args[1].(int))
	case "sendbytes":
		sts.sendLen += uint64(args[1].(int))
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

	rnum := runtime.NumGoroutine()
	msts := runtime.MemStats{}
	runtime.ReadMemStats(&msts)

	wb := bytes.NewBufferString("")
	str := fmt.Sprintf("toxtun stats: %v\n", sts.appmode)

	str = str + fmt.Sprintf("Online Status: self: %v/%v, peer: %v/%v\n",
		sts.selfOnline, sts.selfOfflineCount, sts.peerOnline, sts.peerOfflineCount)
	str = str + fmt.Sprintf("Connections: total: %v, ok: %v, err: %v, active: %v, chans: %v\n",
		sts.connCount, sts.connOKCount, sts.connErrorCount, sts.connActiveCount, sts.chanActiveCount)

	dsrate := float64(0)
	if sts.recvLen > 0 {
		dsrate = float64(sts.sendLen) / float64(sts.recvLen)
	}
	str = str + fmt.Sprintf("DataStream: recv: %v, send: %v, r/s: 1/%.0f\n",
		sts.recvLen, sts.sendLen, dsrate)

	str = str + fmt.Sprintf("goroutines: cur: %v, max: %v, min: %v\n",
		rnum, sts.maxGoRoutine, sts.minGoRoutine)
	str = str + fmt.Sprintf("memory: cur: %v, max: %v, min: %v, total: %v\n",
		msts.Alloc, sts.maxMem, sts.minMem, msts.TotalAlloc)

	str = str + fmt.Sprintf("\nGo internals: %v\n", "")
	str = str + fmt.Sprintf("goroutines: %v\n", rnum)
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
	str = str + "MemStats:\n"
	str = str + string(jstr)
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

	http.HandleFunc("/", indexHandler)
	http.ListenAndServe(":33444", nil)
}

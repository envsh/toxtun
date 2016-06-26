package main

// This logging trick is learnt from a post by Rob Pike
// https://groups.google.com/d/msg/golang-nuts/gU7oQGoCkmg/j3nNxuS2O_sJ

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"runtime"
	"strconv"
	"strings"

	"github.com/cyfdecyf/color"
)

// TODO 最后一条日志的重复归并，
// 即如果连续出现相同的日志，则不再输出更多相同的行，而是对重复的行进行计数修改。
// 这种功能在emacs的*Message*中看到过。
type infoLogging bool
type debugLogging bool
type errorLogging bool
type requestLogging bool
type responseLogging bool

var (
	info   infoLogging
	debug  debugLogging
	errl   errorLogging
	dbgRq  requestLogging
	dbgRep responseLogging

	logFile  io.Writer
	logFlags = log.Flags()

	// make sure logger can be called before initLog
	infoLog     = log.New(os.Stdout, "[INFO] ", logFlags)
	errorLog    = log.New(os.Stdout, "[ERROR] ", logFlags)
	debugLog    = log.New(os.Stdout, "[DEBUG] ", logFlags)
	requestLog  = log.New(os.Stdout, "[>>>>>] ", logFlags)
	responseLog = log.New(os.Stdout, "[<<<<<] ", logFlags)

	verbose  bool
	colorize bool
)

func init() {
	flag.BoolVar((*bool)(&info), "info", true, "info log")
	flag.BoolVar((*bool)(&debug), "debug", false, "debug log, with this option, log goes to stdout with color")
	flag.BoolVar((*bool)(&errl), "err", true, "error log")
	/*
		flag.BoolVar((*bool)(&dbgRq), "request", true, "request log")
		flag.BoolVar((*bool)(&dbgRep), "reply", true, "reply log")
	*/
	flag.BoolVar(&verbose, "v", false, "more info in request/response logging")
	flag.BoolVar(&colorize, "color", false, "colorize log output")
}

func initLog() {
	logFile = os.Stdout
	log.SetOutput(logFile)
	if colorize {
		color.SetDefaultColor(color.ANSI)
	} else {
		color.SetDefaultColor(color.NoColor)
	}
	infoLog = log.New(logFile, "[INFO] ", logFlags)
	errorLog = log.New(logFile, color.Red("[ERROR] "), logFlags)
	debugLog = log.New(logFile, color.Blue("[DEBUG] "), logFlags)
	requestLog = log.New(logFile, color.Green("[>>>>>] "), logFlags)
	responseLog = log.New(logFile, color.Yellow("[<<<<<] "), logFlags)
}

func (d infoLogging) Printf(format string, args ...interface{}) {
	if d {
		format = "%s " + format
		args = append(getFileLine(), args...)
		infoLog.Printf(format, args...)
	}
}

func (d infoLogging) Println(args ...interface{}) {
	if d {
		args = append(getFileLine(), args...)
		infoLog.Println(args...)
	}
}

func (d debugLogging) Printf(format string, args ...interface{}) {
	if d {
		format = "%s " + format
		args = append(getFileLine(), args...)
		debugLog.Printf(format, args...)
	}
}

func (d debugLogging) Println(args ...interface{}) {
	if d {
		args = append(getFileLine(), args...)
		debugLog.Println(args...)
	}
}

func (d errorLogging) Printf(format string, args ...interface{}) {
	if d {
		format = "%s " + format
		args = append(getFileLine(), args...)
		errorLog.Printf(format, args...)
	}
}

func (d errorLogging) Println(args ...interface{}) {
	if d {
		args = append(getFileLine(), args...)
		errorLog.Println(args...)
	}
}

func (d requestLogging) Printf(format string, args ...interface{}) {
	if d {
		format = "%s " + format
		args = append(getFileLine(), args...)
		requestLog.Printf(format, args...)
	}
}

func (d responseLogging) Printf(format string, args ...interface{}) {
	if d {
		format = "%s " + format
		args = append(getFileLine(), args...)
		responseLog.Printf(format, args...)
	}
}

func Fatal(args ...interface{}) {
	fmt.Println(args...)
	os.Exit(1)
}

func Fatalf(format string, args ...interface{}) {
	fmt.Printf(format, args...)
	os.Exit(1)
}

func getFileLine() []interface{} {
	pc, file, line, ok := runtime.Caller(2)
	if !ok {
		file = "unknown"
	}
	fn := runtime.FuncForPC(pc)

	pos := strings.LastIndexByte(file, os.PathSeparator)
	sfile := file[pos+1:]
	return []interface{}{sfile + ":" + strconv.Itoa(line) + " " + fn.Name()}
}

// log folder
var _lastlog string

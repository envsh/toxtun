package main

import (
	"fmt"
	"os"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

/*
func main() {

	info2 = true
	info2.Println("<abcdefg>")
	os.Stdout.Sync()

	// time.Sleep(2 * time.Second)
	info2.Println("<abcdefg>")
	// time.Sleep(2 * time.Second)
	info2.Println("<abcdefg>")
	// time.Sleep(2 * time.Second)

	info2.Println("<abcdefg>c")

	info2.Println("<abcdefg>")
	// time.Sleep(2 * time.Second)
	info2.Println("<abcdefg>")
	// time.Sleep(2 * time.Second)
	fmt.Println()
}
*/

// TODO 最后一条日志的重复归并，
// 即如果连续出现相同的日志，则不再输出更多相同的行，而是对重复的行进行计数修改。
// 这种功能在emacs的*Message*中看到过。
type infoLogger bool
type debugLogger bool
type errorLogger bool
type requestLogger bool
type responseLogger bool

var (
	info2 infoLogger
)

func (d infoLogger) Println(args ...interface{}) {
	if d {
		alog := fmt.Sprint(args...)
		newlog, _needbr := foldLog(alog)

		if _needbr {
			fmt.Print("\n")
		}
		fmt.Print(newlog)
	}
}

// log folder
var _lastlog string

// 折叠相邻并且相同的log
// folder to format: file.go:20 xxx +123
// 相同log按照file:no log计算
func foldLog(alog string) (newlog string, _needbr bool) {
	_needbr = false
	fileline := getFileLine2()
	alog = fileline + " " + alog

	exp := `\+(\d)`
	_trim := func(s string) string {
		return strings.Trim(s, " \t\n")
	}
	same := func(lastlog, curlog string) bool {
		if lastlog == curlog {
			return true
		}

		if strings.HasPrefix(lastlog, curlog) {
			suffix := lastlog[len(curlog):]

			if ok, err := regexp.MatchString(exp, _trim(suffix)); ok && err == nil {
				return true
			}
		}

		return false
	}

	bsfix := ""
	if len(alog) > 0 && len(_lastlog) > 0 && same(_lastlog, alog) {

		if _lastlog == alog {
			bsfix = strings.Repeat("\b", len(alog))
			newlog = alog + " +1"
		} else {
			suffix := _lastlog[len(alog):]
			ok, err := regexp.MatchString(exp, _trim(suffix))
			if err != nil {
			}

			if ok {
				cexp, err := regexp.Compile(exp)
				if err != nil {
				}
				mats := cexp.FindAllStringSubmatch(suffix, -1)
				times, err := strconv.Atoi(mats[0][1])
				bsfix = strings.Repeat("\b", len(_lastlog))
				newlog = alog + " +" + strconv.Itoa(times+1)

			} else {
				newlog = alog
			}
		}
	} else {
		if len(_lastlog) > 0 {
			_needbr = true
		}
		newlog = alog
	}

	_lastlog = newlog

	tfmt := "2006/01/02 15:04:05 "
	now := time.Now().Format(tfmt)
	level := "[DEBUG] "
	newlog = strings.Repeat("\b", len(tfmt)+len(level)) +
		bsfix + level + now + newlog

	return
}

func getFileLine2() string {
	pc, file, line, ok := runtime.Caller(2)
	if !ok {
		file = "unknown"
	}
	fn := runtime.FuncForPC(pc)

	pos := strings.LastIndexByte(file, os.PathSeparator)
	sfile := file[pos+1:]
	return sfile + ":" + strconv.Itoa(line) + " " + fn.Name()
}

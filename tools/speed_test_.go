package main

import (
	"io"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"time"
)

func init() {
	log.SetFlags(log.Flags() | log.Lshortfile)
}

func main() { main_speed_test() }
func main_speed_test() {
	links := []string{
		"https://archive.mozilla.org/pub/firefox/releases/0.10/contrib-localized/firefox-1.0PR.it-IT.langpack.xpi", // 300K

		"https://archive.mozilla.org/pub/firefox/releases/0.10/Firefox%20Setup%201.0PR.exe", // 3M

		"https://archive.mozilla.org/pub/firefox/releases/0.10/firefox-1.0PR-source.tar.bz2", // 30M

		// "http://download.qt.io/official_releases/qt/5.6/5.6.3/single/qt-everywhere-opensource-src-5.6.3.tar.xz",    // 300M

		"https://libreoffice.soluzioniopen.com/stable/standard/LibreOffice-still.standard-x86_64.AppImage", // 300M

		"https://www.mirrorservice.org/sites/download.qt-project.org/official_releases/qt/5.11/5.11.1/qt-opensource-mac-x64-5.11.1.dmg",

		// "https://download.qt.io/archive/qt/5.11/5.11.1/qt-opensource-mac-x64-5.11.1.dmg",                    // 3G
	}
	for i := 0; i < 5; i++ {
		if i > 0 {
			time.Sleep(time.Duration(rand.Int()%9) * time.Second)
		}
		go download(i, links[i])
	}
	select {}
}

func download(tno int, link string) {
	for i := 0; ; i++ {
		log.Println("start downloading...", tno, i, link)
		downloadImpl(tno, i, link)
		log.Println("download done...", tno, i, link)
		time.Sleep(time.Duration(rand.Int()%9) * time.Second)
	}
}

func downloadImpl(tno, times int, link string) {

	tp := &http.Transport{}

	pxyurl := "http://127.0.0.1:8113"
	urlo, err := url.Parse(pxyurl)
	if err != nil {
		log.Panicln(err, tno, times, pxyurl)
	}

	tp.Proxy = http.ProxyURL(urlo)
	cli := &http.Client{}
	cli.Transport = tp

	resp, err := cli.Get(link)
	if err != nil {
		log.Println(err, tno, times, link)
		return
	}
	defer resp.Body.Close()

	sizes := []string{"300K", "3M", "30M", "300M", "3G"}
	var rdtotlen int64
	buf := make([]byte, 8192)
	for {
		rn, err := resp.Body.Read(buf)
		if err != nil {
			if err == io.EOF {
			} else {
				log.Println(err, "tno:", tno, "times:", times, "rdlen", rdtotlen, link)
			}
			break
		}
		rdtotlen += int64(rn)
		if tno == 4 {
			log.Printf("tno: %d, dlidx: %d, rdlen: %d/%d/%s\n", tno, times, rn, rdtotlen, sizes[tno])
		}
	}
}

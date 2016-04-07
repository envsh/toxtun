package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"time"
	"tox"
)

var server = []interface{}{
	// "205.185.116.116", uint16(33445), "A179B09749AC826FF01F37A9613F6B57118AE014D4196A0E1105A98F93A54702",
	"127.0.0.1", uint16(33445), "398C8161D038FD328A573FFAA0F5FAAF7FFDE5E8B4350E7D15E6AFD0B993FC52",
}

var fname string

func makeTox(name string) *tox.Tox {
	fname = fmt.Sprintf("./%s.data", name)
	var nickPrefix = fmt.Sprintf("%s.", name)
	var statusText = fmt.Sprintf("%s of toxtun", name)

	opt := tox.NewToxOptions()
	if tox.FileExist(fname) {
		data, err := ioutil.ReadFile(fname)
		if err != nil {
			log.Println(err)
		} else {
			opt.Savedata_data = data
			opt.Savedata_type = tox.SAVEDATA_TYPE_TOX_SAVE
		}
	}
	port := 33445
	var t *tox.Tox
	for i := 0; i < 7; i++ {
		opt.Tcp_port = uint16(port + i)
		opt.Tcp_port = 0
		t = tox.NewTox(opt)
		if t != nil {
			break
		}
	}
	if t == nil {
		panic(nil)
	}

	// r, err := t.Bootstrap(server[0].(string), server[1].(uint16), server[2].(string))
	// r2, err := t.AddTcpRelay(server[0].(string), server[1].(uint16), server[2].(string))
	// debug.Println("bootstrap:", r, err)

	pubkey := t.SelfGetPublicKey()
	seckey := t.SelfGetSecretKey()
	toxid := t.SelfGetAddress()
	debug.Println("keys:", pubkey, seckey, len(pubkey), len(seckey))
	log.Println("toxid:", toxid)

	defaultName, err := t.SelfGetName()
	humanName := nickPrefix + toxid[0:5]
	if humanName != defaultName {
		t.SelfSetName(humanName)
	}
	humanName, err = t.SelfGetName()
	debug.Println(humanName, defaultName, err)

	defaultStatusText, err := t.SelfGetStatusMessage()
	if defaultStatusText != statusText {
		t.SelfSetStatusMessage(statusText)
	}
	debug.Println(statusText, defaultStatusText, err)

	sz := t.GetSavedataSize()
	sd := t.GetSavedata()
	debug.Println("savedata:", sz, t)
	debug.Println("savedata", len(sd), t)

	err = t.WriteSavedata(fname)
	debug.Println("savedata write:", err)

	// add friend norequest
	fv := t.SelfGetFriendList()
	for _, fno := range fv {
		fid, err := t.FriendGetPublicKey(fno)
		if err != nil {
			debug.Println(err)
		} else {
			t.FriendAddNorequest(fid)
		}
	}
	debug.Println("add friends:", len(fv))

	return t
}

func iterate(t *tox.Tox) {
	// toxcore loops
	shutdown := false
	loopc := 0
	itval := 0
	if !shutdown {
		iv := t.IterationInterval()
		if iv != itval {
			if itval-iv > 20 || iv-itval > 20 {
				// debug.Println("tox itval changed:", itval, iv)
			}
			itval = iv
		}

		t.Iterate()
		status := t.SelfGetConnectionStatus()
		if loopc%5500 == 0 {
			if status == 0 {
				// debug.Printf(".")
			} else {
				// debug.Printf("%d,", status)
			}
		}
		loopc += 1
		time.Sleep(1000 * 50 * time.Microsecond)
	}

	// t.Kill()
}

package main

import (
	"fmt"
	"io/ioutil"
	"time"
	"tox"
)

var server = []interface{}{
	"114.215.156.251", uint16(33445), "4575D94B71E432331BEB8CF5638CD78AD8385EACE76046AD35C440EF51C0D046",
	"205.185.116.116", uint16(33445), "A179B09749AC826FF01F37A9613F6B57118AE014D4196A0E1105A98F93A54702",
	"121.42.190.32", uint16(33445), "0246E8E1DDF5FFCA357E55C6BEA11490E5BFF274D4861DE51E33EA604EFAAA36",
	"biribiri.org", uint16(33445), "F404ABAA1C99A9D37D61AB54898F56793E1DEF8BD46B1038B9D822E8460FAB67",
	"zawertun.net", uint16(33445), "5521952892FBD5C185DF7180DB4DEF69D7844DEEE79B1F75A634ED9DF656756E",
	// "51.254.84.212", uint16(33445), "AEC204B9A4501412D5F0BB67D9C81B5DB3EE6ADA64122D32A3E9B093D544327D",
	// "128.199.199.197", uint16(33445), "B05C8869DBB4EDDD308F43C1A974A20A725A36EACCA123862FDE9945BF9D3E09",
	// "45.78.18.235", uint16(33445), "08E74649D87F67ACF5AEAD020D914215A5521B9FD3C743548B792F16EF3A5D0D",
	// "127.0.0.1", uint16(33445), "398C8161D038FD328A573FFAA0F5FAAF7FFDE5E8B4350E7D15E6AFD0B993FC52",
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
			errl.Println(err)
		} else {
			opt.Savedata_data = data
			opt.Savedata_type = tox.SAVEDATA_TYPE_TOX_SAVE
		}
	}
	port := 33445
	var t *tox.Tox
	for i := 0; i < 7; i++ {
		opt.Tcp_port = uint16(port)
		// opt.Tcp_port = 0
		t = tox.NewTox(opt)
		if t != nil {
			break
		}
		port += 1
	}
	if t == nil {
		panic(nil)
	}
	info.Println("TCP port:", opt.Tcp_port)
	if false {
		time.Sleep(1 * time.Hour)
	}

	for i := 0; i < len(server)/3; i++ {
		r := i * 3
		r1, err := t.Bootstrap(server[r+0].(string), server[r+1].(uint16), server[r+2].(string))
		r2, err := t.AddTcpRelay(server[r+0].(string), server[r+1].(uint16), server[r+2].(string))
		info.Println("bootstrap:", r1, err, r2, i, r)
	}

	pubkey := t.SelfGetPublicKey()
	seckey := t.SelfGetSecretKey()
	toxid := t.SelfGetAddress()
	debug.Println("keys:", pubkey, seckey, len(pubkey), len(seckey))
	info.Println("toxid:", toxid)

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
		// time.Sleep(50 * time.Millisecond)
	}

	// t.Kill()
}

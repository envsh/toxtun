package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"sort"
	"time"

	// "github.com/kitech/go-toxcore"
	tox "github.com/TokTok/go-toxcore-c"
	"github.com/bitly/go-simplejson"
)

var servers = []interface{}{
	"114.215.156.251", uint16(33445), "4575D94B71E432331BEB8CF5638CD78AD8385EACE76046AD35C440EF51C0D046",
	"205.185.116.116", uint16(33445), "A179B09749AC826FF01F37A9613F6B57118AE014D4196A0E1105A98F93A54702",
	"121.42.190.32", uint16(33445), "0246E8E1DDF5FFCA357E55C6BEA11490E5BFF274D4861DE51E33EA604EFAAA36",
	"biribiri.org", uint16(33445), "F404ABAA1C99A9D37D61AB54898F56793E1DEF8BD46B1038B9D822E8460FAB67",
	"zawertun.net", uint16(33445), "5521952892FBD5C185DF7180DB4DEF69D7844DEEE79B1F75A634ED9DF656756E",
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
			log.Println(lerrorp, err)
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
	log.Println(linfop, "TCP port:", opt.Tcp_port)
	if false {
		time.Sleep(1 * time.Hour)
	}

	for i := 0; i < len(servers)/3; i++ {
		r := i * 3
		r1, err := t.Bootstrap(servers[r+0].(string), servers[r+1].(uint16), servers[r+2].(string))
		r2, err := t.AddTcpRelay(servers[r+0].(string), servers[r+1].(uint16), servers[r+2].(string))
		log.Println(linfop, "bootstrap:", r1, err, r2, i, r)
	}

	pubkey := t.SelfGetPublicKey()
	seckey := t.SelfGetSecretKey()
	toxid := t.SelfGetAddress()
	log.Println(ldebugp, "keys:", pubkey, seckey, len(pubkey), len(seckey))
	log.Println(linfop, "toxid:", toxid)

	defaultName := t.SelfGetName()
	humanName := nickPrefix + toxid[0:5]
	if humanName != defaultName {
		t.SelfSetName(humanName)
	}
	humanName = t.SelfGetName()
	log.Println(ldebugp, humanName, defaultName)

	defaultStatusText, err := t.SelfGetStatusMessage()
	if defaultStatusText != statusText {
		t.SelfSetStatusMessage(statusText)
	}
	log.Println(ldebugp, statusText, defaultStatusText, err)

	sz := t.GetSavedataSize()
	sd := t.GetSavedata()
	log.Println(ldebugp, "savedata:", sz, t)
	log.Println(ldebugp, "savedata", len(sd), t)

	err = t.WriteSavedata(fname)
	log.Println(ldebugp, "savedata write:", err)

	// add friend norequest
	fv := t.SelfGetFriendList()
	for _, fno := range fv {
		fid, err := t.FriendGetPublicKey(fno)
		if err != nil {
			log.Println(lerrorp, err)
		} else {
			t.FriendAddNorequest(fid)
		}
	}
	log.Println(ldebugp, "add friends:", len(fv))

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

// 切换到其他的bootstrap nodes上
func switchServer(t *tox.Tox) {
	newNodes := get3nodes()
	for _, node := range newNodes {
		r1, err := t.Bootstrap(node.ipaddr, node.port, node.pubkey)
		if node.status_tcp {
			r2, err := t.AddTcpRelay(node.ipaddr, node.port, node.pubkey)
			log.Println(linfop, "bootstrap(tcp):", r1, err, r2, node.ipaddr, node.last_ping, node.status_tcp)
		} else {
			log.Println(linfop, "bootstrap(udp):", r1, err, node.ipaddr,
				node.last_ping, node.status_tcp, node.last_ping_rt)
		}
	}
	currNodes = newNodes
}

func get3nodes() (nodes [3]ToxNode) {
	idxes := make(map[int]bool, 0)
	currips := make(map[string]bool, 0)
	for idx := 0; idx < len(currNodes); idx++ {
		currips[currNodes[idx].ipaddr] = true
	}
	for n := 0; n < len(allNodes)*3; n++ {
		idx := rand.Int() % len(allNodes)
		_, ok1 := idxes[idx]
		_, ok2 := currips[allNodes[idx].ipaddr]
		if !ok1 && !ok2 && allNodes[idx].status_tcp == true && allNodes[idx].last_ping_rt > 0 {
			idxes[idx] = true
			if len(idxes) == 3 {
				break
			}
		}
	}
	if len(idxes) < 3 {
		log.Println(lerrorp, "can not find 3 new nodes:", idxes)
	}

	_idx := 0
	for k, _ := range idxes {
		nodes[_idx] = allNodes[k]
		_idx += 1
	}
	return
}

func init() {
	rand.Seed(time.Now().UnixNano())
	initThirdPartyNodes()
	initToxNodes()
	// go pingNodes()
}

// fixme: chown root.root toxtun-go && chmod u+s toxtun-go
// should block
func pingNodes() {
	stop := false
	for !stop {
		btime := time.Now()
		errcnt := 0
		for idx, node := range allNodes {
			if false {
				log.Println(idx, node)
			}
			if true {
				// rtt, err := Ping0(node.ipaddr, 3)
				rtt, err := Ping0(node.ipaddr, 3)
				if err != nil {
					// log.Println("ping", ok, node.ipaddr, rtt.String())
					log.Println("ping", err, node.ipaddr, rtt.String())
					errcnt += 1
				}
				if err == nil {
					allNodes[idx].last_ping_rt = uint(time.Now().Unix())
					allNodes[idx].rtt = rtt
				} else {
					allNodes[idx].last_ping_rt = uint(0)
					allNodes[idx].rtt = time.Duration(0)
				}
			}
		}
		etime := time.Now()
		log.Printf("Pinged all=%d, errcnt=%d, %v\n", len(allNodes), errcnt, etime.Sub(btime))

		// TODO longer ping interval
		time.Sleep(30 * time.Second)
	}
}

func initThirdPartyNodes() {
	for idx := 0; idx < 3*3; idx += 3 {
		node := ToxNode{
			isthird:      true,
			ipaddr:       servers[idx].(string),
			port:         servers[idx+1].(uint16),
			pubkey:       servers[idx+2].(string),
			last_ping:    uint(time.Now().Unix()),
			last_ping_rt: uint(time.Now().Unix()),
			status_tcp:   true,
		}

		allNodes = append(allNodes, node)
	}
}

func initToxNodes() {
	bcc, err := Asset("toxnodes.json")
	if err != nil {
		log.Panicln(err)
	}
	jso, err := simplejson.NewJson(bcc)
	if err != nil {
		log.Panicln(err)
	}

	nodes := jso.Get("nodes").MustArray()
	for idx := 0; idx < len(nodes); idx++ {
		nodej := jso.Get("nodes").GetIndex(idx)
		/*
			log.Println(idx, nodej.Get("ipv4"), nodej.Get("port"), nodej.Get("last_ping"),
				len(nodej.Get("tcp_ports").MustArray()))
		*/
		node := ToxNode{
			ipaddr:       nodej.Get("ipv4").MustString(),
			port:         uint16(nodej.Get("port").MustUint64()),
			pubkey:       nodej.Get("public_key").MustString(),
			last_ping:    uint(nodej.Get("last_ping").MustUint64()),
			status_tcp:   nodej.Get("status_tcp").MustBool(),
			last_ping_rt: uint(time.Now().Unix()),
			weight:       calcNodeWeight(nodej),
		}

		allNodes = append(allNodes, node)
		if idx < len(currNodes) {
			currNodes[idx] = node
		}
	}

	sort.Sort(ByRand(allNodes))
	for idx, node := range allNodes {
		if false {
			log.Println(idx, node.ipaddr, node.port, node.last_ping)
		}
	}
	log.Println(linfop, "Load nodes:", len(allNodes))
}

func calcNodeWeight(nodej *simplejson.Json) int {
	return 0
}

var allNodes = make([]ToxNode, 0)
var currNodes [3]ToxNode

type ToxNode struct {
	isthird    bool
	ipaddr     string
	port       uint16
	pubkey     string
	weight     int
	usetimes   int
	legacy     int
	chktimes   int
	last_ping  uint
	status_tcp bool
	///
	last_ping_rt uint // 程序内ping的时间
	rtt          time.Duration
}

type ByRand []ToxNode

func (this ByRand) Len() int           { return len(this) }
func (this ByRand) Swap(i, j int)      { this[i], this[j] = this[j], this[i] }
func (this ByRand) Less(i, j int) bool { return rand.Int()%2 == 0 }

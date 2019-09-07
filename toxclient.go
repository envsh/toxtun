package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"strings"
	"time"

	tox "github.com/TokTok/go-toxcore-c"
	"github.com/envsh/go-toxcore/xtox"
)

var cn_servers = []interface{}{
	"116.196.105.228", uint16(33445), "F5273784AAD79F46BE7CEAE7E7E92EDCF799963DE3F4E244A19E139A1CBF911D",
	"119.23.239.31", uint16(33445), "7F613A23C9EA5AC200264EB727429F39931A86C39B67FC14D9ECA4EBE0D37F25", // CN HangZhou1
	"120.79.155.35", uint16(33445), "1A00BCD1F0E0E5BC08B0F4B4934B08BCB02028D6D2F127C3441E06FE659A993F",
	"116.196.77.132", uint16(33445), "040326E850DDCB49B1B2D9E3E2789D425774E4C5D783A55C09A024D05D2A8A66", // CN Beijing
	"103.74.192.222", uint16(33445), "8A7F2FB2A5C83BCFD677DC0513F8EA8E41D14A9EFFB86BFAD3C2E51E1BCB0F04", // CN HK
	// tox.yikifish.com
	// "tox.yikifish.com", uint16(33445), "8EF12E275BA9CD7D56625D4950F2058B06D5905D0650A1FE76AF18DB986DF760",
	// "47.91.166.18", uint16(33556), "BEB842FDF490F9EF9F7453E8DABDFF3929AB33F4490287182E8DA6EDAC3CFA18", // CN HK xx
	// "free.idcfengye.com", uint16(17501), "2F0683A8AA6F29B2E043E5423073C7F89F662D3777FE85615963E97EF8AF2803",
	// "39.108.221.234", uint16(33445), "0E81B1927B25EB7A4857C36FE5D6B938841676ECFEA5557D1CFB7DF4A946967F", // CN HangZhou2
	// "10.0.0.7", uint16(33345), "2F0683A8AA6F29B2E043E5423073C7F89F662D3777FE85615963E97EF8AF2803",
}
var ru_servers = []interface{}{
	// RU good at midnight
	// no tcp
	// "92.54.84.70", uint16(33445), "5625A62618CB4FCA70E147A71B29695F38CC65FF0CBD68AD46254585BE564802",
	"85.172.30.117", uint16(33445), "8E7D0B859922EF569298B4D261A8CCB5FEA14FB91ED412A7603A585A25698832",
	// "80.87.193.193", uint16(33445), "B38255EE4B054924F6D79A5E6E5889EC94B6ADF6FE9906F97A3D01E3D083223A",
	"91.234.60.90", uint16(33445), "EEDE4F1ADB8C2D3A7DFF9A9C23BF7E3AB0FEFF6C98A0C3879FBD5059FE1ABE14",
	"81.177.26.122", uint16(33445), "FB6A7FFE8F144B3ACBD00B7C644AFA14F8764DFADE6DA5691965C7F45A604450",
	// "79.140.30.52", uint16(33445), "FFAC871E85B1E1487F87AE7C76726AE0E60318A85F6A1669E04C47EB8DC7C72D",
	"195.91.228.210", uint16(33445), "7467AFA626D3246343170B309BA5BDC975DF3924FC9D7A5917FBFA9F5CD5CD38",
	"95.31.18.227", uint16(33445), "257744DBF57BE3E117FE05D145B5F806089428D4DCE4E3D0D50616AA16D9417E",
	"t0x-node1.weba.ru", uint16(33445), "5A59705F86B9FC0671FDF72ED9BB5E55015FF20B349985543DDD4B0656CA1C63",
	// 300+ms
	// "tox-node.loskiq.it", uint16(33445), "88124F3C18C6CFA8778B7679B7329A333616BD27A4DFB562D476681315CF143D",
	// "62.173.139.200", uint16(33445), "4E608965D9BDA877B47ABAF046E58C9C22EE87A07C6B023104F963A1513C3B08", // x
}
var us_servers = []interface{}{
	// US west
	"node.tox.biribiri.org", uint16(33445), "F404ABAA1C99A9D37D61AB54898F56793E1DEF8BD46B1038B9D822E8460FAB67", // 67.215.253.85
	"104.217.252.207", uint16(33445), "C1520BCFCA158ED487861E992D6A4F6025C0F5F170DF958849B44AD28591E476",
	"52.53.185.100", uint16(33445), "A04F5FE1D006871588C8EC163676458C1EC75B20B4A147433D271E1E85DAF839",

	// US east
	// "104.223.122.15", uint16(33445), "0FB96EEBFB1650DDB52E70CF773DDFCABE25A95CC3BB50FC251082E4B63EF82A",
	// 	"205.185.116.116", uint16(33445), "A179B09749AC826FF01F37A9613F6B57118AE014D4196A0E1105A98F93A54702",
	// "198.98.51.198", uint16(33445), "1D5A5F2F5D6233058BF0259B09622FB40B482E4FA0931EB8FD3AB8E7BF7DAF6F",

	// "51.254.84.212", uint16(33445), "AEC204B9A4501412D5F0BB67D9C81B5DB3EE6ADA64122D32A3E9B093D544327D", // x
	// "127.0.0.1", uint16(33445), "398C8161D038FD328A573FFAA0F5FAAF7FFDE5E8B4350E7D15E6AFD0B993FC52",
}
var servers = []interface{}{}

var tox_savedata_fname string
var tox_disable_udp = false
var tox_bs_group = "" // us,ru,cn,auto

func init() {
	flag.BoolVar(&tox_disable_udp, "disable-udp", tox_disable_udp,
		fmt.Sprintf("if tox disable udp, default: %v", tox_disable_udp))
	flag.BoolVar(&useFixedBSs, "use-fixedbs", useFixedBSs, "use fixed bootstraps, possible faster.")
	flag.StringVar(&tox_bs_group, "bs-group", tox_bs_group, "bootstrap group, us,ru,cn,auto")

	add_our_nodes(cn_servers, "cn")
	add_our_nodes(us_servers, "us")
	add_our_nodes(ru_servers, "ru")
}

func add_our_nodes(nodes []interface{}, grp string) {
	for i := 0; i < len(nodes)/3; i++ {
		r := i * 3
		ipstr, port, pubkey := nodes[r+0].(string), nodes[r+1].(uint16), nodes[r+2].(string)
		xtox.AddNode(pubkey, ipstr, int(port), grp, int(port))
	}
}

func is_selected_server(pubkey string) bool {
	for j := 0; j < len(servers)/3; j++ {
		if pubkey == servers[j*3+2].(string) {
			return true
		}
	}
	return false
}

// mode: client|server
func set_bootstrap_group(mode string) {
	tmpsrvs := append(append(append([]interface{}{}, cn_servers...), us_servers...), ru_servers...)
	bsgroups := map[string]interface{}{"us": us_servers, "ru": ru_servers, "cn": cn_servers, "auto": tmpsrvs}
	switch mode {
	case "client", "inone":
		if bsgroupx, ok := bsgroups[tox_bs_group]; ok {
			// servers = bsgroupx.([]interface{})[:9]
			servers = bsgroupx.([]interface{})
			// servers = []interface{}{"91.234.60.90", uint16(33445), "EEDE4F1ADB8C2D3A7DFF9A9C23BF7E3AB0FEFF6C98A0C3879FBD5059FE1ABE14"}
			log.Println(len(servers), servers)
		} else {
			log.Fatalln("unknown bs group:", tox_bs_group)
		}
	case "server": // server连接全部，由客户端选择连接哪些节点
		servers = tmpsrvs
		servers = cn_servers
	default:
		log.Fatalln("not supported mode:", mode)
	}
}

func makeTox(name string) *tox.Tox {
	tox_savedata_fname = fmt.Sprintf("./%s.data", name)
	var nickPrefix = fmt.Sprintf("%s.", name)
	var statusText = fmt.Sprintf("%s of toxtun", name)

	opt := tox.NewToxOptions()
	opt.Udp_enabled = !tox_disable_udp

	if tox.FileExist(tox_savedata_fname) {
		data, err := ioutil.ReadFile(tox_savedata_fname)
		if err != nil {
			errl.Println(err)
		} else {
			opt.Savedata_data = data
			opt.Savedata_type = tox.SAVEDATA_TYPE_TOX_SAVE
		}
	}
	port := 33445
	var t *tox.Tox
	for i := 0; i < 71; i++ {
		opt.Tcp_port = uint16(port + i)
		// opt.Tcp_port = 0
		t = tox.NewTox(opt)
		if t != nil {
			break
		}
	}
	if t == nil {
		panic(nil)
	}
	if tox_disable_udp {
		info.Println("TCP port:", opt.Tcp_port)
	} else {
		info.Println("TCP port:", "disabled")
	}
	if false {
		time.Sleep(1 * time.Hour)
	}

	for i := 0; i < len(servers)/3; i++ {
		if useFixedBSs {
			// continue
		}
		r := i * 3
		ipstr, port, pubkey := servers[r+0].(string), servers[r+1].(uint16), servers[r+2].(string)
		r1, err := t.Bootstrap(ipstr, port, pubkey)
		r2, err := t.AddTcpRelay(ipstr, port, pubkey)
		info.Println("bootstrap:", r1, err, r2, i, r, ipstr, port)
	}
	if useFixedBSs {
		addFixedBootstraps(t)
	}

	pubkey := t.SelfGetPublicKey()
	seckey := t.SelfGetSecretKey()
	toxid := t.SelfGetAddress()
	debug.Println("keys:", pubkey, seckey, len(pubkey), len(seckey))
	info.Println("toxid:", toxid)

	defaultName := t.SelfGetName()
	humanName := nickPrefix + toxid[0:5]
	if humanName != defaultName {
		t.SelfSetName(humanName)
	}
	humanName = t.SelfGetName()
	debug.Println(humanName, defaultName)

	defaultStatusText, err := t.SelfGetStatusMessage()
	if defaultStatusText != statusText {
		t.SelfSetStatusMessage(statusText)
	}
	debug.Println(statusText, defaultStatusText, err)

	sz := t.GetSavedataSize()
	sd := t.GetSavedata()
	debug.Println("savedata:", sz, t)
	debug.Println("savedata", len(sd), t)

	err = t.WriteSavedata(tox_savedata_fname)
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
	debug.Println("added friends:", len(fv))

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

func toxiter(tox *tox.Tox) {
	tptime := time.Now()
	for {
		time.Sleep(time.Duration(smuse.tox_interval) * time.Millisecond)
		// time.Sleep(30 * time.Millisecond)
		// this.toxPollChan <- ToxPollEvent{}
		outit := false
		if time.Since(tptime) > 10*time.Second {
			tptime = time.Now()
			outit = true
		}
		if outit {
			// log.Println("tox ittttttttttt ...")
		}
		iterate(tox)
		if outit {
			// log.Println("tox ittttttttttt")
		}
	}
}

var useFixedBSs = false

func addFixedBootstraps(t *tox.Tox) {
	if useFixedBSs {
		if false {
			node := []interface{}{"133.130.127.155", uint16(33445), "5EE85FD7B4B6BD8FD113A1E8CC5853A233008B574E07F2CC76A7EA43AE24AE07"}
			_, err := t.Bootstrap(node[0].(string), node[1].(uint16), node[2].(string))
			//_, err = t.AddTcpRelay(node[0].(string), node[1].(uint16), node[2].(string))
			log.Println("hehehe", err == nil)
		}
	}
}

// 切换到其他的bootstrap nodes上
func switchServer(t *tox.Tox) {
	xtox.SwitchServer(t, "cn")

	addFixedBootstraps(t)
}

func init() {
	rand.Seed(time.Now().UnixNano())
}

var livebots = []string{
	"56A1ADE4B65B86BCD51CC73E2CD4E542179F47959FE3E0E21B4B0ACDADE51855D34D34D37CB5", // groupbot
	"76518406F6A9F2217E8DC487CC783C25CC16A15EB36FF32E335A235342C48A39218F515C39A6", //echobot@toxme.io
	"DD7A68B345E0AA918F3544AA916B5CA6AED6DE80389BFF1EF7342DACD597943D62BDEED1FC67", // my echobot
	"03F47F0AE26BE32C73579CBA2C5421A159EDFF74535A7E8C6480398D93A0EA2E02B1B20B80D7", // DobroBot
	"A922A51E1C91205B9F7992E2273107D47C72E8AE909C61C28A77A4A2A115431B14592AB38A3B", // toxirc
	"5EE85FD7B4B6BD8FD113A1E8CC5853A233008B574E07F2CC76A7EA43AE24AE0754DBD6B8FD3F", // ToxIRCBotCN
	"415732B8A549B2A1F9A278B91C649B9E30F07330E8818246375D19E52F927C57F08A44E082F6", // LainBot
	"398C8161D038FD328A573FFAA0F5FAAF7FFDE5E8B4350E7D15E6AFD0B993FC529FA90C343627", // envoy
}

func addLiveBots(t *tox.Tox) {
	for _, botid := range livebots {
		t.FriendAdd(botid, "hello")
	}
}

func livebotsOnFriendConnectionStatus(t *tox.Tox, friendNumber uint32, status int) {
	fid, _ := t.FriendGetPublicKey(friendNumber)
	if strings.HasPrefix(livebots[5], fid) {
		t.FriendSendMessage(friendNumber, "/mute on")
	}
}

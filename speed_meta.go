package main

import (
	"encoding/binary"
	"fmt"
	"gopp"
	"log"
	"math/rand"
	"sort"
	"time"
	"unsafe"

	"github.com/envsh/go-toxcore/mintox"
	metrics "github.com/rcrowley/go-metrics"
)

type SpeedItem struct {
	Key                   string
	Name                  string
	RoundTripTime         int // RTT ms
	BottleneckBandwidth   int // BB
	BandwidthDelayProduct int // BDP
	WriteWndSize          int // write window size
	ReconnCount           int
	Pinid                 uint64
	LastPingTime          time.Time // last ping time
	LastUseTime           time.Time

	Weight int

	mtreg     metrics.Registry
	mtRTT     metrics.Meter // to peer's rtt value
	mtBB      metrics.Meter
	mtPDB     metrics.Meter
	mtWndSize metrics.Meter
	mtReconn  metrics.Counter
	mtRTT2Srv metrics.Timer // to server's rtt value

	bbPktTime   time.Time // send bb test pkts time
	bbPktSeq    uint64
	bbPktSize   int
	bbPktCnt    int // bindwidth test pktid count
	bbPktPreCnt int // want cnt, dynamic adjust
}

func newSpeedItem(key, name string, mtreg metrics.Registry) *SpeedItem {
	this := &SpeedItem{}
	this.Key, this.Name = key, name
	this.mtreg = mtreg

	this.BottleneckBandwidth = 1000 // default value
	this.RoundTripTime = 1000       // default value

	this.mtRTT = metrics.GetOrRegisterMeter(fmt.Sprintf("RTT.%s", this.Name), this.mtreg)
	this.mtBB = metrics.GetOrRegisterMeter(fmt.Sprintf("BB.%s", this.Name), this.mtreg)
	this.mtPDB = metrics.GetOrRegisterMeter(fmt.Sprintf("PDB.%s", this.Name), this.mtreg)
	this.mtWndSize = metrics.GetOrRegisterMeter(fmt.Sprintf("WndSize.%s", this.Name), this.mtreg)
	this.mtReconn = metrics.GetOrRegisterCounter(fmt.Sprintf("Reconn.%s", this.Name), this.mtreg)
	return this
}

//

type SpeedMeta struct {
	items map[string]*SpeedItem
}

func (this *MTox) onReservedData(object mintox.Object, number uint32, connid uint8, data []byte, cbdata mintox.Object) {
	tcpcli := object.(*mintox.TCPClient)
	log.Println(ldebugp, number, connid, len(data), tcpcli.ServAddr)
}

func (this *MTox) daemonProc() {
	stop := false
	ntime := 2
	for !stop {
		time.Sleep(time.Duration(3*ntime) * time.Second)
		this.calcPriority()

		time.Sleep(time.Duration(17*ntime) * time.Second)
		// app ping
		// bindwidth test
		this.clismu.RLock()
		for binpk, clinfo := range this.clis {
			if clinfo.status != 2 {
				continue
			}
			this.sendRTTPing(binpk, clinfo)
			// break
		}
		this.clismu.RUnlock()
		time.Sleep(time.Duration(7*ntime) * time.Second)

		this.clismu.RLock()
		for binpk, clinfo := range this.clis {
			if clinfo.status != 2 {
				continue
			}
			if clinfo.inuse && int(time.Since(clinfo.spditm.LastUseTime).Seconds()) < 61 {
				continue
			}
			this.sendBoundWidthData(binpk, clinfo)
			// time.Sleep(2 * time.Second)
			// break
		}
		this.clismu.RUnlock()
		time.Sleep(time.Duration(3*ntime) * time.Second)
	}
	log.Println("done")
}

var (
	TCP_PACKET_RTTPING = []byte("RTTPING")
	TCP_PACKET_RTTPONG = []byte("RTTPONG")
	TCP_PACKET_BBTREQU = []byte("BBTREQU")
	TCP_PACKET_BBTRESP = []byte("BBTRESP")
	TCP_PACKET_TUNCTRL = []byte("TUNCTRL")
	TCP_PACKET_TUNDATA = []byte("TUNDATA")
)

func (this *MTox) sendRTTPing(binpk string, clinfo *ClientInfo) {
	tcpcli := clinfo.tcpcli

	ping_plain := gopp.NewBufferZero()
	ping_plain.Write(TCP_PACKET_RTTPING)
	pingid := rand.Uint64()
	pingid = gopp.IfElse(pingid == 0, uint64(1), pingid).(uint64)

	clinfo.spditm.LastPingTime = time.Now()
	binary.Write(ping_plain, binary.BigEndian, pingid)
	log.Println("rtt ping plnpkt len:", ping_plain.Len(), pingid)

	connid := clinfo.connid
	buf := gopp.NewBufferZero()
	buf.WriteByte(byte(connid))
	buf.Write(ping_plain.Bytes())

	_, err := tcpcli.SendCtrlPacket(buf.Bytes())
	gopp.ErrPrint(err, pingid)
}

func (this *MTox) sendRTTPong(binpk string, tcpcli *mintox.TCPClient, pingpkt []byte) {
	pongpkt := gopp.NewBufferZero()
	clinfo := this.clis[tcpcli.ServPubkey.BinStr()]
	pongpkt.WriteByte(clinfo.connid)
	pongpkt.Write(TCP_PACKET_RTTPONG)
	pongpkt.Write(pingpkt[len(TCP_PACKET_RTTPING):])
	_, err := tcpcli.SendCtrlPacket(pongpkt.Bytes())
	gopp.ErrPrint(err)
}

func (this *MTox) sendBBTResp(tcpcli *mintox.TCPClient, reqpkt []byte) {
	pongpkt := gopp.NewBufferZero()
	clinfo := this.clis[tcpcli.ServPubkey.BinStr()]
	// pongpkt.WriteByte(clinfo.connid)
	pongpkt.Write(TCP_PACKET_BBTRESP)
	pongpkt.Write(reqpkt[len(TCP_PACKET_RTTPING) : len(TCP_PACKET_RTTPING)+int(unsafe.Sizeof(uint64(0)))])
	// _, err := tcpcli.SendCtrlPacket(pongpkt.Bytes())
	_, err := tcpcli.SendDataPacket(clinfo.connid, pongpkt.Bytes())
	if false {
		gopp.ErrPrint(err)
	}
}

// The harmonic mean
func calchmval(oldr float64, curr float64, n float64) float64 {
	return (n + 1.0) / (n/oldr + 1.0/(curr*(n+1.0)))
}

func (this *MTox) handleBBTResponse(tcpcli *mintox.TCPClient, data []byte) {
	buf := gopp.NewBufferBuf(data[len(TCP_PACKET_BBTRESP):])
	var bbtPktSeq uint64
	binary.Read(buf, binary.BigEndian, &bbtPktSeq)
	if false {
		log.Println("recv bbt ack:", bbtPktSeq)
	}
	clinfo := this.clis[tcpcli.ServPubkey.BinStr()]
	spditm := clinfo.spditm
	if bbtPktSeq == spditm.bbPktSeq {
		spditm.bbPktCnt--
		if spditm.bbPktCnt == 0 {
			spdval := float64(spditm.bbPktSize) / time.Since(spditm.bbPktTime).Seconds()
			hmspdval := calchmval(float64(spditm.BottleneckBandwidth), spdval, float64(bbtPktSeq))
			spditm.BottleneckBandwidth = int(hmspdval)
			log.Println("recv bbt done:", bbtPktSeq, spditm.bbPktSize,
				time.Since(spditm.bbPktTime), int(spdval), spditm.Name)
			timen := int(time.Since(spditm.bbPktTime).Seconds())
			if timen > 5 {
				oldcnt := spditm.bbPktPreCnt
				spditm.bbPktPreCnt = spditm.bbPktPreCnt*5/timen + 4
				log.Println("use too long for test, adjust:", oldcnt, "=>", spditm.bbPktPreCnt, spditm.Name)
			}
		}
	} else {
		// too late pkt
	}
}

func (this *MTox) sendBoundWidthData(binpk string, clinfo *ClientInfo) {
	tcpcli := clinfo.tcpcli
	connid := clinfo.connid
	spditm := clinfo.spditm

	// last time test is not finished
	if spditm.bbPktPreCnt > 0 && spditm.bbPktCnt > 0 {
		spditm.bbPktPreCnt = spditm.bbPktPreCnt / 2
	}

	// TODO best send 3-5s data
	cnt := gopp.IfElseInt(spditm.bbPktPreCnt == 0, 31, spditm.bbPktPreCnt)
	cnt = gopp.IfElseInt(cnt < 16, 16, cnt)
	pktsz := 987
	spditm.bbPktTime = time.Now()
	spditm.bbPktSeq += 1
	spditm.bbPktSize = int(unsafe.Sizeof(uint64(0)))*cnt + pktsz*cnt
	spditm.bbPktCnt = cnt
	rdstr := gopp.RandomStringPrintable(pktsz)
	pkt := gopp.NewBufferZero()
	// pkt.WriteByte(connid)
	pkt.Write(TCP_PACKET_BBTREQU)
	binary.Write(pkt, binary.BigEndian, spditm.bbPktSeq)
	pkt.Write([]byte(rdstr))
	for i := 0; i < cnt; i++ {
		buf := gopp.BytesDup(pkt.Bytes()) // !!!
		// _, err := tcpcli.SendCtrlPacket(buf)
		_, err := tcpcli.SendDataPacket(connid, buf)
		if err != nil {
			leftn := cnt - i
			spditm.bbPktSize -= int(unsafe.Sizeof(uint64(0)))*leftn + pktsz*leftn
			spditm.bbPktCnt -= leftn
			break
		}
	}

	if spditm.bbPktCnt >= cnt/2 {
		log.Println("sent bbtest data:", spditm.bbPktSeq, spditm.bbPktCnt, cnt, spditm.bbPktSize, spditm.Name)
		spditm.bbPktPreCnt = spditm.bbPktCnt + 16
		spditm.bbPktPreCnt = gopp.IfElseInt(spditm.bbPktPreCnt > 200, 200, spditm.bbPktPreCnt)
	} else {
		log.Println("sent bbtest data failed of less than half pkt:",
			spditm.bbPktSeq, spditm.bbPktCnt, cnt, spditm.bbPktSize, spditm.Name)
		spditm.bbPktPreCnt = spditm.bbPktCnt //  (cnt+ spditm.bbPktCnt) / 2
		spditm.bbPktSeq = 0
		spditm.bbPktCnt = 0
		spditm.bbPktSize = 0
	}
}

func (this *MTox) calcPriority() {
	var spditms []*SpeedItem
	this.clismu.RLock()
	for _, clinfo := range this.clis {
		if clinfo.status != 2 {
			continue
		}
		spditms = append(spditms, clinfo.spditm)
	}
	this.clismu.RUnlock()

	this.clismu.RLock()
	for _, itm := range spditms {
		rtt := int(itm.RoundTripTime)
		rtt = gopp.IfElseInt(rtt == 0, 100, rtt)
		rtt += 100 // +100稀释RTT的效果
		weight := itm.BottleneckBandwidth * 1000 / rtt
		itm.Weight = weight
		itm.BandwidthDelayProduct = weight
		itm.WriteWndSize = itm.BandwidthDelayProduct / 8
	}
	this.clismu.RUnlock()

	sort.Slice(spditms, func(i int, j int) bool {
		return spditms[i].Weight > spditms[j].Weight
		// return spditms[i].BottleneckBandwidth > spditms[j].BottleneckBandwidth
	})

	keys := []string{}
	this.clismu.RLock()
	for i, itm := range spditms {
		log.Printf("%d, name: %s, BB: %d, RTT: %d/%d, weight: %d, wndsz: %d\n",
			i, itm.Name, itm.BottleneckBandwidth, itm.RoundTripTime, int(itm.mtRTT.Rate5()), itm.Weight, itm.WriteWndSize)
		if len(keys) < 3 {
			keys = append(keys, itm.Key)
			this.clis[itm.Key].inuse = true
		} else {
			this.clis[itm.Key].inuse = false
		}
		if i > 81 {
			break
		}
	}
	this.clismu.RUnlock()
	this.clismu.Lock()
	oldkeys := this.relays
	this.relays = keys
	changed := !EqualNoOrder(oldkeys, keys)
	this.clismu.Unlock()

	// check change
	log.Println("selected relays changed:", changed, sltchgcnt)
	sltchgcnt++
}

var sltchgcnt int = 0

func EqualNoOrder(s1, s2 []string) bool {
	if len(s1) != len(s2) {
		return false
	}
	for _, e1 := range s1 {
		found := false
		for _, e2 := range s2 {
			if e2 == e1 {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

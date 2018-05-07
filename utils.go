package main

import (
	"fmt"
	"time"

	"net"
)

func iclock() uint32 {
	return uint32((time.Now().UnixNano() / 1000000) & 0xffffffff)
}

// 这里优先级：外网IP、内网IP、loopback
func getOutboundIp() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		errl.Println(err, addrs)
	}

	// 查找公网IP
	for _, addr := range addrs {
		aip := addr.(*net.IPNet).IP
		if !aip.IsLoopback() && !isReservedIp(aip) {
			// 现在还是需要排除IPv6
			if len(aip.String()) > len("255.255.255.255") {
			} else {
				return aip.String()
			}
		}
	}

	// 查找内网IP
	for _, addr := range addrs {
		aip := addr.(*net.IPNet).IP
		if !aip.IsLoopback() && isReservedIp(aip) {
			return aip.String()
		}
	}

	// 查找loopback IP
	for _, addr := range addrs {
		aip := addr.(*net.IPNet).IP
		if aip.IsLoopback() {
			return aip.String()
		}
	}

	return ""
}

// func init() { info.Println(getOutboundIp()) }
func ip2long(ip net.IP) uint32 {
	return uint32(ip[0])<<24 | uint32(ip[1])<<16 | uint32(ip[2])<<8 | uint32(ip[3])
}

func isReservedIp(ip net.IP) bool {
	// ip range
	// 10.0.0.0--10.255.255.255
	// 172.16.0.0--172.31.255.255
	// 192.168.0.0--192.168.255.255
	rips := []net.IP{
		net.ParseIP("10.0.0.0"),
		net.ParseIP("10.255.255.255"),
		net.ParseIP("172.16.0.0"),
		net.ParseIP("172.31.255.255"),
		net.ParseIP("192.168.0.0"),
		net.ParseIP("192.168.255.255"),
	}

	ipn := ip2long(ip)

	for idx := 0; idx < len(rips); idx += 2 {
		ipc0 := ip2long(rips[idx])
		ipc1 := ip2long(rips[idx+1])
		if ipn >= ipc0 && ipn <= ipc1 {
			return true
		}
	}
	return false
}

func isReservedIpStr(ip string) bool {
	return isReservedIp(net.ParseIP(ip))
}

func ListenUdpPortAuto(bport int, host string) (net.PacketConn, error) {
	var pc net.PacketConn
	var err error
	for port := bport; port < 65536; port++ {
		pc, err = net.ListenPacket("udp", fmt.Sprintf("%s:%d", host, port))
		if err == nil {
			return pc, nil
		}
	}
	return nil, err
}

func kcp_reset_by_newest_rcvbuf(kcp *KCP) *KCP {
	debug.Println(len(kcp.rcv_buf), len(kcp.rcv_queue))
	kcpnxt := kcp.rcv_nxt
	segsn := kcp.rcv_buf[len(kcp.rcv_buf)-1].sn
	if kcpnxt == 0 && segsn > 0 {
		kcp.rcv_nxt = segsn
	} else if kcpnxt > 0 && segsn == 0 {
		kcp.rcv_nxt = segsn
	}
	return kcp
}

func kcp_reset_by_seg(kcp *KCP, seg *segment) *KCP {
	debug.Println(len(kcp.rcv_buf), len(kcp.rcv_queue))
	kcpnxt := kcp.rcv_nxt
	segsn := seg.sn
	if kcpnxt == 0 && segsn > 0 {
		kcp.rcv_nxt = segsn
	} else if kcpnxt > 0 && segsn == 0 {
		kcp.rcv_nxt = segsn
		kcp.rcv_buf = kcp.rcv_buf[:0]
		kcp.rcv_queue = kcp.rcv_queue[:0]
	}
	return kcp
}

func kcp_parse_segment(buf []byte) *segment {
	data := make([]byte, 32)
	copy(data, buf)

	var ts, sn, length, una, conv uint32
	var wnd uint16
	var cmd, frg uint8

	data = ikcp_decode32u(data, &conv)

	data = ikcp_decode8u(data, &cmd)
	data = ikcp_decode8u(data, &frg)
	data = ikcp_decode16u(data, &wnd)
	data = ikcp_decode32u(data, &ts)
	data = ikcp_decode32u(data, &sn)
	data = ikcp_decode32u(data, &una)
	data = ikcp_decode32u(data, &length)

	seg := &segment{}
	seg.conv = conv
	seg.cmd = cmd
	seg.frg = frg
	seg.wnd = wnd
	seg.ts = ts
	seg.sn = sn
	seg.una = una

	return seg
}

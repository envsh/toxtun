package main

import (
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

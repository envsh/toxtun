package main

//

// NoDelay options
// fastest: ikcp_nodelay(kcp, 1, 20, 2, 1)
// nodelay: 0:disable(default), 1:enable
// interval: internal update timer interval in millisec, default is 100ms
// resend: 0:disable fast resend(default), 1:enable fast resend
// nc: 0:normal congestion control(default), 1:disable congestion control
/*
kcp_mode_default: []int{0, 50, 0, 0},
kcp_mode_normal: []int{0, 40, 2, 1},
kcp_mode_fast:   []int{0, 30, 2, 1},
kcp_mode_fast2:  []int{1, 20, 2, 1},
kcp_mode_fast3: []int{1, 10, 2, 1},
*/

type SpeedMode struct {
	name         string
	tox_interval int
	kcp_interval int

	nodelay, interval, resend, nc int

	wndsz int
}

// fast3 > fast2 > fast > normal > default
// like tcp
const tox_interval = 200
const kcp_interval = 75 // 50-100

var SMTcp = &SpeedMode{name: "default-tcp", tox_interval: tox_interval, kcp_interval: kcp_interval,
	nodelay: 0, interval: 100, resend: 0, nc: 0, wndsz: 128}
var SMDefault = SMTcp

var SMTcp1 = &SpeedMode{name: "tcp1", tox_interval: tox_interval, kcp_interval: kcp_interval,
	nodelay: 0, interval: 50, resend: 0, nc: 0, wndsz: 128}

var SMTcp2 = &SpeedMode{name: "tcp2", tox_interval: tox_interval, kcp_interval: kcp_interval,
	nodelay: 0, interval: 20, resend: 0, nc: 0, wndsz: 128}

var SMTcp3 = &SpeedMode{name: "tcp3", tox_interval: tox_interval, kcp_interval: kcp_interval,
	nodelay: 0, interval: 10, resend: 0, nc: 0, wndsz: 128}

var SMTcp4 = &SpeedMode{name: "tcp4", tox_interval: tox_interval, kcp_interval: kcp_interval,
	nodelay: 0, interval: 200, resend: 0, nc: 0, wndsz: 256}

var SMTcp5 = &SpeedMode{name: "tcp5", tox_interval: tox_interval, kcp_interval: kcp_interval,
	nodelay: 0, interval: 100, resend: 0, nc: 1, wndsz: 256}

var SMTcp6 = &SpeedMode{name: "tcp6", tox_interval: tox_interval, kcp_interval: kcp_interval,
	nodelay: 1, interval: 75, resend: 0, nc: 1, wndsz: 256}

var SMTcp7 = &SpeedMode{name: "tcp7", tox_interval: tox_interval, kcp_interval: kcp_interval,
	nodelay: 1, interval: 100, resend: 0, nc: 1, wndsz: 128}

var SMTcp9 = &SpeedMode{name: "tcp9", tox_interval: tox_interval, kcp_interval: kcp_interval,
	nodelay: 1, interval: kcp_interval*2 + 50, resend: 0, nc: 1, wndsz: 386} //512?

var SMTcp10 = &SpeedMode{name: "tcp10", tox_interval: tox_interval, kcp_interval: kcp_interval,
	nodelay: 1, interval: kcp_interval*3 + 50, resend: 0, nc: 1, wndsz: 512} //512?

var SMTcp11 = &SpeedMode{name: "tcp11", tox_interval: tox_interval, kcp_interval: kcp_interval / 2,
	nodelay: 1, interval: kcp_interval*3 + 50, resend: 0, nc: 1, wndsz: 512} //512?

var SMTcp12 = &SpeedMode{name: "tcp12", tox_interval: tox_interval, kcp_interval: kcp_interval / 2,
	nodelay: 1, interval: kcp_interval*4 + 50, resend: 0, nc: 1, wndsz: 512} //512?

var SMTcp13 = &SpeedMode{name: "tcp13", tox_interval: tox_interval, kcp_interval: kcp_interval / 3,
	nodelay: 1, interval: kcp_interval*5 + 50, resend: 0, nc: 1, wndsz: 512} //512?

var SMTcp14 = &SpeedMode{name: "tcp14", tox_interval: tox_interval, kcp_interval: kcp_interval / 10,
	nodelay: 1, interval: kcp_interval*5 + 50, resend: 0, nc: 1, wndsz: 512} //512?

var SMTcp15 = &SpeedMode{name: "tcp15", tox_interval: tox_interval, kcp_interval: kcp_interval / 20,
	nodelay: 1, interval: kcp_interval*5 + 50, resend: 0, nc: 1, wndsz: 512} //512?

var SMTcp16 = &SpeedMode{name: "tcp16", tox_interval: tox_interval, kcp_interval: kcp_interval / 25,
	nodelay: 1, interval: kcp_interval*5 + 50, resend: 0, nc: 1, wndsz: 512} //512?

var SMTcp17 = &SpeedMode{name: "tcp17", tox_interval: tox_interval, kcp_interval: kcp_interval / 25,
	nodelay: 1, interval: kcp_interval*3 + 50, resend: 0, nc: 1, wndsz: 512} //512?

var SMTcp18 = &SpeedMode{name: "tcp18", tox_interval: tox_interval, kcp_interval: kcp_interval / 10,
	nodelay: 1, interval: kcp_interval*2 + 50, resend: 0, nc: 1, wndsz: 512} //512?

// interval太小，重传量大
var SMTcp19 = &SpeedMode{name: "tcp19", tox_interval: tox_interval, kcp_interval: kcp_interval / 10,
	nodelay: 1, interval: kcp_interval + 50, resend: 0, nc: 1, wndsz: 512} //512?

// interval太小，重传量大。速度能达到最大，重传量也还行基本在2倍重传
var SMTcp20 = &SpeedMode{name: "tcp20", tox_interval: tox_interval, kcp_interval: 10,
	nodelay: 1, interval: 100, resend: 0, nc: 1, wndsz: 512} //512?

var SMNormal = &SpeedMode{name: "normal", tox_interval: tox_interval, kcp_interval: kcp_interval,
	nodelay: 0, interval: 50, resend: 0, nc: 0, wndsz: 32}

var SMFast = &SpeedMode{name: "fast", tox_interval: tox_interval, kcp_interval: kcp_interval,
	nodelay: 1, interval: 40, resend: 2, nc: 1, wndsz: 64}

var SMFast2 = &SpeedMode{name: "fast2", tox_interval: tox_interval, kcp_interval: kcp_interval,
	nodelay: 1, interval: 20, resend: 2, nc: 1, wndsz: 96}

var SMFast3 = &SpeedMode{name: "fast3", tox_interval: tox_interval, kcp_interval: kcp_interval,
	nodelay: 1, interval: 10, resend: 2, nc: 1, wndsz: 128}

var smuse_default = *SMTcp20
var smuse = smuse_default

func set_speed_mode(mode string) {
	switch mode {
	default:
		info.Printf("%s mode not found, use default\n", mode)
		fallthrough
	case "default":
		// smuse = *SMTcp
	case "tcp1":
		smuse = *SMTcp1
	case "tcp2":
		smuse = *SMTcp2
	case "tcp3":
		smuse = *SMTcp3
	case "tcp4":
		smuse = *SMTcp4
	case "tcp5":
		smuse = *SMTcp5
	case "tcp6":
		smuse = *SMTcp6
	case "tcp7":
		smuse = *SMTcp7
	case "tcp9":
		smuse = *SMTcp9
	case "tcp10":
		smuse = *SMTcp10
	case "tcp11":
		smuse = *SMTcp11
	case "tcp12":
		smuse = *SMTcp12
	case "tcp13":
		smuse = *SMTcp13
	case "tcp14":
		smuse = *SMTcp14
	case "tcp15":
		smuse = *SMTcp15
	case "tcp16":
		smuse = *SMTcp16
	case "tcp17":
		smuse = *SMTcp17
	case "tcp18":
		smuse = *SMTcp18
	case "tcp19":
		smuse = *SMTcp19
	case "tcp20":
		smuse = *SMTcp20
	case "normal":
		smuse = *SMNormal
	case "fast":
		smuse = *SMFast
	case "fast2":
		smuse = *SMFast2
	case "fast3":
		smuse = *SMFast3
	}

	info.Println("switch to kcp-mode:", smuse.name)
}

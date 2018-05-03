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
}

// fast3 > fast2 > fast > normal > default
// like tcp
const tox_interval = 200
const kcp_interval = 75 // 50-100

var SMTcp = &SpeedMode{name: "default-tcp", tox_interval: tox_interval, kcp_interval: kcp_interval,
	nodelay: 0, interval: 100, resend: 0, nc: 0}
var SMDefault = SMTcp

var SMNormal = &SpeedMode{name: "normal", tox_interval: tox_interval, kcp_interval: kcp_interval,
	nodelay: 0, interval: 50, resend: 0, nc: 0}

var SMFast = &SpeedMode{name: "fast", tox_interval: tox_interval, kcp_interval: kcp_interval,
	nodelay: 1, interval: 40, resend: 2, nc: 1}

var SMFast2 = &SpeedMode{name: "fast2", tox_interval: tox_interval, kcp_interval: kcp_interval,
	nodelay: 1, interval: 20, resend: 2, nc: 1}

var SMFast3 = &SpeedMode{name: "fast3", tox_interval: tox_interval, kcp_interval: kcp_interval,
	nodelay: 1, interval: 10, resend: 2, nc: 1}

var smuse = SMTcp

func set_speed_mode(mode string) {
	switch mode {
	case "default":
		fallthrough
	default:
		smuse = SMTcp
	case "normal":
		smuse = SMNormal
	case "fast":
		smuse = SMFast
	case "fast2":
		smuse = SMFast2
	case "fast3":
		smuse = SMFast3
	}
}

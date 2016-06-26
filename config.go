package main

import (
	"strconv"
	"strings"

	"github.com/go-ini/ini"
)

const (
// toxtunid = "A90BE3636C169E5991C910950E90C7524C83460971F6D710D6AC0591B8A7B62235B0BE180AC5"
)

type TunnelRecord struct {
	lhost   string
	lport   int
	rhost   string
	rport   int
	rpubkey string
}

type TunnelConfig struct {
	cfg_file string
	recs     []TunnelRecord
	srv_name string
	sets     *ini.File
}

func NewTunnelConfig(cfg_file string) *TunnelConfig {
	f, err := ini.Load(cfg_file)
	if err != nil {
		errl.Println(err)
		return nil
	}

	recs := make([]TunnelRecord, 0)

	srv_val, err := f.Section("server").GetKey("name")
	srv_name := srv_val.String()

	for _, key := range f.Section("client").KeyStrings() {
		cli_val, err := f.Section("client").GetKey(key)
		if err != nil {
		}
		line := cli_val.String()

		rec := parseRecordLine(line)
		recs = append(recs, rec)
	}

	return &TunnelConfig{cfg_file, recs, srv_name, f}
}

func parseRecordLine(line string) TunnelRecord {
	sep := ":"
	segs := strings.Split(line, sep)

	lhost := segs[0]
	lport, err := strconv.Atoi(segs[1])
	if err != nil {
	}
	rhost := segs[2]
	rport, err := strconv.Atoi(segs[3])
	rpubkey := segs[4]
	if len(rpubkey) != 76 {
	}

	return TunnelRecord{
		lhost, lport, rhost, rport, rpubkey,
	}
}

func friendInConfig(pubkey string) bool {
	for _, rec := range config.recs {
		if pubkey == rec.rpubkey || strings.HasPrefix(rec.rpubkey, pubkey) {
			return true
		}
	}

	return false
}

package main

import (
	"log"
	"strconv"
	"strings"

	"github.com/go-ini/ini"
)

const (
// toxtunid = "A90BE3636C169E5991C910950E90C7524C83460971F6D710D6AC0591B8A7B62235B0BE180AC5"
)

var (
	// from config_manual.go
	outboundip = outboundip_const
	mpcsz      = 256
)

type TunnelRecord struct {
	lhost   string
	lport   int
	rhost   string
	rport   int
	rpubkey string
	tname   string
	tproto  string // TCP/UDP
}

type TunnelConfig struct {
	cfg_file string
	recs     map[string]*TunnelRecord
	srv_name string
	sets     *ini.File
}

func NewTunnelConfig(cfg_file string) *TunnelConfig {
	f, err := ini.Load(cfg_file)
	if err != nil {
		log.Println(lerrorp, err)
		return nil
	}

	recs := make(map[string]*TunnelRecord, 0)

	srv_val, err := f.Section("server").GetKey("name")
	srv_name := srv_val.String()

	for _, key := range f.Section("client").KeyStrings() {
		cli_val, err := f.Section("client").GetKey(key)
		if err != nil {
		}
		line := cli_val.String()

		rec := parseRecordLine(line)
		rec.tname = key
		if _, ok := recs[key]; ok {
			// already exist
		}
		recs[key] = &rec
	}

	return &TunnelConfig{cfg_file, recs, srv_name, f}
}

func parseRecordLine(line string) TunnelRecord {
	sep := ":"
	segs := strings.Split(line, sep)

	tproto := strings.ToUpper(segs[0])
	lhost := segs[1]
	lport, err := strconv.Atoi(segs[2])
	if err != nil {
	}
	rhost := segs[3]
	rport, err := strconv.Atoi(segs[4])
	rpubkey := segs[5]
	if len(rpubkey) != 76 {
	}

	return TunnelRecord{
		lhost, lport, rhost, rport, rpubkey, "", tproto,
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

func (this *TunnelConfig) friendInConfig(pubkey string) bool {
	for _, rec := range config.recs {
		if pubkey == rec.rpubkey || strings.HasPrefix(rec.rpubkey, pubkey) {
			return true
		}
	}

	return false
}

func (this *TunnelConfig) getRecordByName(tname string) *TunnelRecord {
	if rec, ok := this.recs[tname]; ok {
		return rec
	}
	return nil
}

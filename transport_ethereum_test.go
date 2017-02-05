package main

import (
	"log"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
)

func TestEthAddr(t *testing.T) {
	prikey, err := crypto.GenerateKey()
	if err != nil {
		log.Println(err)
	} else {
		pubaddr := crypto.PubkeyToAddress(prikey.PublicKey).Hex()
		log.Println(pubaddr)

		pubkey_bin := crypto.FromECDSAPub(&prikey.PublicKey)
		log.Println(len(pubkey_bin), pubkey_bin)

		newpubkey1 := crypto.ToECDSAPub(pubkey_bin)
		log.Println(*newpubkey1 == prikey.PublicKey, &prikey.PublicKey)
		log.Println(*newpubkey1 == prikey.PublicKey, newpubkey1)

		newpubkey := crypto.ToECDSAPub([]byte(pubaddr))
		log.Println(*newpubkey == prikey.PublicKey, newpubkey)
	}
}

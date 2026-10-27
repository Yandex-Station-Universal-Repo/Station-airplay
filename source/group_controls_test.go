package main

import (
	"bytes"
	"net/netip"
	"testing"
	"time"
)

func TestGroupAuthenticationFreshnessReplayAndSource(t *testing.T) {
	key := bytes.Repeat([]byte{9}, 32)
	own := netip.MustParseAddr("10.0.0.10")
	peer := netip.MustParseAddr("10.0.0.11")
	makeVerifier := func() *groupVerifier {
		return &groupVerifier{key: key, own: own, peers: map[netip.Addr]bool{peer: true}}
	}
	gid, e := uuidBytes("00112233-4455-6677-8899-aabbccddeeff")
	if e != nil {
		t.Fatal(e)
	}
	now := time.Unix(1700000000, 0)
	nonce := [16]byte{1}
	b, e := groupPacket(key, peer, gid, 65, now, nonce)
	if e != nil {
		t.Fatal(e)
	}
	v := makeVerifier()
	m, e := v.verify(b, peer, now)
	if e != nil || m.percent != 65 || m.group != gid {
		t.Fatal(m, e)
	}
	if _, e = v.verify(b, peer, now); e == nil {
		t.Fatal("replay accepted")
	}
	for _, offset := range []time.Duration{-2 * time.Second, 4 * time.Second} {
		if _, e = makeVerifier().verify(b, peer, now.Add(offset)); e == nil {
			t.Fatal("future/expired packet accepted")
		}
	}
	corrupt := append([]byte(nil), b...)
	corrupt[45] = 99
	if _, e = makeVerifier().verify(corrupt, peer, now); e == nil {
		t.Fatal("tampered HMAC accepted")
	}
	if _, e = makeVerifier().verify(b, own, now); e == nil {
		t.Fatal("sender spoof accepted")
	}
	if _, e = makeVerifier().verify(b, netip.MustParseAddr("10.0.0.12"), now); e == nil {
		t.Fatal("unknown peer accepted")
	}
	for _, pct := range []int{-1, 101} {
		if _, e = groupPacket(key, peer, gid, pct, now, nonce); e == nil {
			t.Fatal(pct)
		}
	}
	if _, e = makeVerifier().verify(append(b, 0), peer, now); e == nil {
		t.Fatal("trailing bytes accepted")
	}
}
func TestGroupOrderingUUIDAndCapacity(t *testing.T) {
	key := bytes.Repeat([]byte{5}, 32)
	peer := netip.MustParseAddr("10.0.0.11")
	own := netip.MustParseAddr("10.0.0.10")
	now := time.Unix(1700000000, 0)
	gid, _ := uuidBytes("00112233-4455-6677-8899-aabbccddeeff")
	v := groupVerifier{key: key, own: own, peers: map[netip.Addr]bool{peer: true}}
	for i := 0; i < 256; i++ {
		var nonce [16]byte
		nonce[0] = byte(i)
		b, _ := groupPacket(key, peer, gid, i%101, now, nonce)
		if _, e := v.verify(b, peer, now); e != nil {
			t.Fatal(i, e)
		}
	}
	b, _ := groupPacket(key, peer, gid, 50, now, [16]byte{0, 1})
	if _, e := v.verify(b, peer, now); e == nil {
		t.Fatal("unbounded replay cache")
	}
	b, _ = groupPacket(key, peer, gid, 50, now.Add(4*time.Second), [16]byte{0, 2})
	if _, e := v.verify(b, peer, now.Add(4*time.Second)); e != nil {
		t.Fatal("expired cache did not retire", e)
	}
	b, _ = groupPacket(key, peer, gid, 50, now.Add(3*time.Second), [16]byte{0, 3})
	if _, e := v.verify(b, peer, now.Add(4*time.Second)); e == nil {
		t.Fatal("older command accepted")
	}
	for _, s := range []string{"", "00112233445566778899aabbccddeeff", "00112233-4455-6677-8899-aabbccddeefg"} {
		if _, e := uuidBytes(s); e == nil {
			t.Fatal(s)
		}
	}
}

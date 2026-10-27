package main

// Optional peer volume bridge. Explicit LAN peers, private key, bounded freshness
// and replay protection, and fresh OWN live AirPlay group checks on both ends.
import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io"
	"net"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"time"
)

type groupConfig struct {
	Bind    string   `json:"bind"`
	Peers   []string `json:"peers"`
	KeyFile string   `json:"key_file"`
}
type groupMessage struct {
	group   [16]byte
	percent int
	at      int64
	nonce   [16]byte
	source  netip.Addr
}
type groupVerifier struct {
	key   []byte
	own   netip.Addr
	peers map[netip.Addr]bool
	seen  map[string]int64
	last  map[netip.Addr]int64
}

func uuidBytes(value string) (out [16]byte, err error) {
	if len(value) != 36 || value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' {
		return out, errors.New("invalid group UUID")
	}
	b, e := hex.DecodeString(strings.ReplaceAll(value, "-", ""))
	if e != nil || len(b) != 16 {
		return out, errors.New("invalid group UUID")
	}
	copy(out[:], b)
	return out, nil
}
func groupPacket(key []byte, source netip.Addr, gid [16]byte, pct int, now time.Time, nonce [16]byte) ([]byte, error) {
	if len(key) != 32 || !source.Is4() || pct < 0 || pct > 100 {
		return nil, errors.New("invalid group packet arguments")
	}
	b := make([]byte, 78)
	b[0] = 1
	ip := source.As4()
	copy(b[1:5], ip[:])
	copy(b[5:21], gid[:])
	binary.BigEndian.PutUint64(b[21:29], uint64(now.UnixMilli()))
	copy(b[29:45], nonce[:])
	b[45] = byte(pct)
	mac := hmac.New(sha256.New, key)
	mac.Write(b[:46])
	copy(b[46:], mac.Sum(nil))
	return b, nil
}
func freshGroup(at int64, now time.Time) bool {
	return at >= now.Add(-3*time.Second).UnixMilli() && at <= now.Add(time.Second).UnixMilli()
}
func (v *groupVerifier) verify(b []byte, source netip.Addr, now time.Time) (groupMessage, error) {
	m := groupMessage{}
	if len(b) != 78 || b[0] != 1 || !v.peers[source] || source == v.own {
		return m, errors.New("untrusted group source")
	}
	var ip [4]byte
	copy(ip[:], b[1:5])
	if netip.AddrFrom4(ip) != source {
		return m, errors.New("group source mismatch")
	}
	mac := hmac.New(sha256.New, v.key)
	mac.Write(b[:46])
	if !hmac.Equal(mac.Sum(nil), b[46:]) {
		return m, errors.New("group signature rejected")
	}
	m.source = source
	m.at = int64(binary.BigEndian.Uint64(b[21:29]))
	m.percent = int(b[45])
	copy(m.group[:], b[5:21])
	copy(m.nonce[:], b[29:45])
	if !freshGroup(m.at, now) || m.percent > 100 {
		return m, errors.New("stale or invalid group command")
	}
	if v.seen == nil {
		v.seen = make(map[string]int64)
		v.last = make(map[netip.Addr]int64)
	}
	for k, at := range v.seen {
		if !freshGroup(at, now) {
			delete(v.seen, k)
		}
	}
	k := source.String() + hex.EncodeToString(m.nonce[:])
	if _, ok := v.seen[k]; ok {
		return m, errors.New("group replay rejected")
	}
	if m.at < v.last[source] || len(v.seen) >= 256 {
		return m, errors.New("out-of-order group command or replay capacity reached")
	}
	v.seen[k] = m.at
	v.last[source] = m.at
	return m, nil
}

type groupBridge struct {
	conn     *net.UDPConn
	verifier groupVerifier
	name     string
	receiver *dbusReceiver
}

func openGroup(cfg groupConfig, name string, r *dbusReceiver) (*groupBridge, error) {
	ip, err := netip.ParseAddr(cfg.Bind)
	if err != nil || !ip.Is4() || !ip.IsPrivate() {
		return nil, errors.New("group bind must be an explicit private LAN IPv4 address")
	}
	if len(cfg.Peers) == 0 || len(cfg.Peers) > 32 {
		return nil, errors.New("group requires1..32 explicit peers")
	}
	peers := map[netip.Addr]bool{}
	for _, peer := range cfg.Peers {
		p, e := netip.ParseAddr(peer)
		if e != nil || !p.Is4() || !p.IsPrivate() || p == ip || peers[p] {
			return nil, errors.New("invalid or duplicate group peer")
		}
		peers[p] = true
	}
	f, e := os.Open(cfg.KeyFile)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	st, e := f.Stat()
	if e != nil || !st.Mode().IsRegular() || st.Mode().Perm()&0077 != 0 || st.Size() != 32 {
		return nil, errors.New("group key must be a private regular32-byte file")
	}
	key := make([]byte, 32)
	if _, e = io.ReadFull(f, key); e != nil {
		return nil, e
	}
	conn, e := net.ListenUDP("udp4", net.UDPAddrFromAddrPort(netip.AddrPortFrom(ip, 43792)))
	if e != nil {
		return nil, e
	}
	return &groupBridge{conn: conn, verifier: groupVerifier{key: key, own: ip, peers: peers}, name: name, receiver: r}, nil
}
func (b *groupBridge) membership(ctx context.Context) ([16]byte, error) {
	var iface, proto, addressProto int32
	var name, kind, domain, host, address string
	var port uint16
	var txt [][]byte
	var flags uint32
	err := b.receiver.conn.Object("org.freedesktop.Avahi", "/").CallWithContext(ctx, "org.freedesktop.Avahi.Server.ResolveService", 0,
		int32(-1), int32(0), b.name, "_airplay._tcp", "local", int32(0), uint32(0)).Store(&iface, &proto, &name, &kind, &domain, &host, &addressProto, &address, &port, &txt, &flags)
	if err != nil {
		return [16]byte{}, err
	}
	if address != b.verifier.own.String() || flags&12 != 12 || name != b.name || kind != "_airplay._tcp" || domain != "local" {
		return [16]byte{}, errors.New("group is not OWN live AirPlay service")
	}
	var gid string
	var status uint64
	seen := map[string]bool{}
	for _, raw := range txt {
		key, value, ok := strings.Cut(string(raw), "=")
		if !ok {
			continue
		}
		if (key == "gid" || key == "flags") && seen[key] {
			return [16]byte{}, errors.New("duplicate group TXT")
		}
		seen[key] = true
		switch key {
		case "gid":
			gid = value
		case "flags":
			status, err = strconv.ParseUint(value, 0, 64)
			if err != nil {
				return [16]byte{}, err
			}
		}
	}
	if status&(1<<17) == 0 {
		return [16]byte{}, errors.New("AirPlay group is idle")
	}
	return uuidBytes(gid)
}
func (b *groupBridge) broadcast(ctx context.Context, pct int) error {
	gid, e := b.membership(ctx)
	if e != nil {
		return e
	}
	var nonce [16]byte
	if _, e = rand.Read(nonce[:]); e != nil {
		return e
	}
	packet, e := groupPacket(b.verifier.key, b.verifier.own, gid, pct, time.Now(), nonce)
	if e != nil {
		return e
	}
	for peer := range b.verifier.peers {
		if _, e = b.conn.WriteToUDPAddrPort(packet, netip.AddrPortFrom(peer, 43792)); e != nil {
			return e
		}
	}
	return nil
}
func (b *groupBridge) listen(ctx context.Context, events chan<- groupMessage) {
	buf := make([]byte, 79)
	for ctx.Err() == nil {
		_ = b.conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		n, peer, e := b.conn.ReadFromUDPAddrPort(buf)
		if e != nil {
			if ctx.Err() != nil {
				return
			}
			if ne, ok := e.(net.Error); ok && ne.Timeout() {
				continue
			}
			return
		}
		if peer.Port() != 43792 {
			continue
		}
		m, e := b.verifier.verify(buf[:n], peer.Addr(), time.Now())
		if e != nil {
			continue
		}
		select {
		case events <- m:
		default:
		} // no stale deferred work when the receiver is busy.
	}
}
func (c *sharedController) group(ctx context.Context, b *groupBridge, m groupMessage, now time.Time) error {
	if !c.cfg.NativeVolume || !c.active || now.Before(c.duckUntil) || !freshGroup(m.at, now) {
		return nil
	}
	s, e := c.receiver.State(ctx)
	if e != nil {
		return e
	}
	if !s.Active || !s.Available || s.Client != c.client {
		return nil
	}
	gid, e := b.membership(ctx)
	if e != nil {
		return e
	}
	if gid != m.group {
		return nil
	}
	actual, e := c.station.SetVolume(ctx, m.percent)
	if e != nil {
		return e
	}
	pct, e := nativePercent(actual)
	if e != nil {
		return e
	}
	if e = c.receiver.Volume(ctx, percentDB(pct)); e != nil {
		c.volumeReady = false
		return e
	}
	c.lastNative = pct
	c.lastDB = percentDB(pct)
	c.remotePending = true
	c.remoteOldDB = s.DB
	c.remoteDeadline = now.Add(3 * time.Second)
	c.pendingNative = -1
	return nil // received changes are never re-broadcast.
}

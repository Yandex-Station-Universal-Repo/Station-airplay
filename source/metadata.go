// GPL-3.0-or-later; adapted from airplayd 1.1.3-plum.
package main

import (
	"encoding/base64"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// Preserved from the verified airplayd 1.1.3-plum metadata parser.
type progress struct {
	have, paused bool
	seconds      float64
	at           time.Time
	length       float64
}

func (p *progress) update(raw string, now time.Time) bool {
	pieces := strings.Split(strings.TrimSpace(raw), "/")
	if len(pieces) != 3 {
		return false
	}
	var t [3]uint32
	for i, x := range pieces {
		u, e := strconv.ParseUint(x, 10, 32)
		if e != nil {
			return false
		}
		t[i] = uint32(u)
	}
	total := uint32(t[2] - t[0])
	passed := uint32(t[1] - t[0])
	// 32-bit RTP timestamp wrap is allowed; reject impossible ordering or durations.
	if total == 0 || passed > total || total > 24*3600*44100 {
		return false
	}
	p.length = float64(total) / 44100.0
	p.seconds = float64(total-passed) / 44100.0
	p.have = true
	p.at = now
	p.paused = false
	return true
}
func (p *progress) remaining(now time.Time) (int, bool) {
	if !p.have {
		return 0, false
	}
	rem := p.seconds
	if !p.paused {
		elapsed := now.Sub(p.at).Seconds()
		if elapsed > 0 {
			rem -= elapsed
		}
	}
	if rem < 0 {
		rem = 0
	}
	if rem > 24*3600 {
		return 0, false
	}
	return int(math.Ceil(rem)), true
}
func (p *progress) pause(now time.Time) {
	if !p.have || p.paused {
		return
	}
	elapsed := now.Sub(p.at).Seconds()
	if elapsed > 0 {
		p.seconds -= elapsed
	}
	if p.seconds < 0 {
		p.seconds = 0
	}
	p.at = now
	p.paused = true
}
func (p *progress) resume(now time.Time) {
	if p.have {
		p.paused = false
		p.at = now
	}
}

// Transliteration is intentionally lossy: seven-segment LEDs cannot render
// arbitrary Cyrillic or Latin, but Latin approximations are scannable.
var translit = map[rune]string{
	'А': "A", 'Б': "B", 'В': "V", 'Г': "G", 'Д': "D", 'Е': "E", 'Ё': "YO", 'Ж': "ZH", 'З': "Z", 'И': "I", 'Й': "I", 'К': "K", 'Л': "L", 'М': "M", 'Н': "N", 'О': "O", 'П': "P", 'Р': "R", 'С': "S", 'Т': "T", 'У': "U", 'Ф': "F", 'Х': "H", 'Ц': "TS", 'Ч': "CH", 'Ш': "SH", 'Щ': "SCH", 'Ъ': "", 'Ы': "Y", 'Ь': "", 'Э': "E", 'Ю': "YU", 'Я': "YA",
	'І': "I", 'Ї': "YI", 'Є': "YE", 'Ґ': "G", 'Ў': "U",
}

func scrollText(input string) []rune {
	input = strings.TrimSpace(input)
	if input == "" {
		input = "NO TITLE"
	}
	var b strings.Builder
	b.Grow(len(input) + 8)
	previousSpace := true
	for _, c := range input {
		if b.Len() > 200 {
			break
		}
		c = unicode.ToUpper(c)
		if x, ok := translit[c]; ok {
			b.WriteString(x)
			previousSpace = false
			continue
		}
		if c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' {
			b.WriteRune(c)
			previousSpace = false
			continue
		}
		if unicode.IsSpace(c) || unicode.IsPunct(c) || unicode.IsSymbol(c) {
			if !previousSpace {
				b.WriteByte(' ')
				previousSpace = true
			}
			continue
		}
		// Unknown glyph is a separator, never raw uncontrolled display input.
		if !previousSpace {
			b.WriteByte(' ')
			previousSpace = true
		}
	}
	return []rune("    " + strings.TrimSpace(b.String()) + "    ")
}

type metadataParser struct{ buffer []byte }

func tag(s, open, close string) (string, bool) {
	a := strings.Index(s, open)
	if a < 0 {
		return "", false
	}
	a += len(open)
	b := strings.Index(s[a:], close)
	if b < 0 {
		return "", false
	}
	return s[a : a+b], true
}
func parseItem(item string) (typ, code string, payload []byte, ok bool) {
	typ, ok = tag(item, "<type>", "</type>")
	if !ok {
		return
	}
	code, ok = tag(item, "<code>", "</code>")
	if !ok {
		return
	}
	ls, ok := tag(item, "<length>", "</length>")
	if !ok {
		return "", "", nil, false
	}
	length, e := strconv.Atoi(ls)
	if e != nil || length < 0 || length > 2048 {
		return "", "", nil, false
	}
	if length == 0 {
		return typ, code, nil, true
	}
	raw, ok := tag(item, "<data encoding=\"base64\">", "</data>")
	if !ok {
		return "", "", nil, false
	}
	raw = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == ' ' || r == '\t' {
			return -1
		}
		return r
	}, raw)
	payload, e = base64.StdEncoding.Strict().DecodeString(raw)
	if e != nil || len(payload) != length {
		return "", "", nil, false
	}
	return typ, code, payload, true
}
func (p *metadataParser) feed(data []byte, visit func(string, string, []byte)) {
	p.buffer = append(p.buffer, data...)
	for {
		open := strings.Index(string(p.buffer), "<item>")
		if open < 0 {
			if len(p.buffer) > 5 {
				p.buffer = append([]byte(nil), p.buffer[len(p.buffer)-5:]...)
			}
			return
		}
		if open > 0 {
			p.buffer = p.buffer[open:]
		}
		end := strings.Index(string(p.buffer), "</item>")
		if end < 0 {
			if len(p.buffer) > 16384 {
				p.buffer = p.buffer[:0]
			}
			return
		}
		raw := string(p.buffer[:end+7])
		p.buffer = p.buffer[end+7:]
		typ, code, payload, ok := parseItem(raw)
		if ok {
			visit(typ, code, payload)
		}
	}
}

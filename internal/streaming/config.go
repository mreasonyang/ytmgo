package streaming

import (
	"encoding/json"
	"errors"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

var configCalls = regexp.MustCompile(`\bytcfg\.set\s*\(\s*`)
var challengeCalls = regexp.MustCompile(`\bwindow\.ytAtN\s*\(\s*`)
var globalNamePattern = regexp.MustCompile(`^[A-Za-z_$][A-Za-z0-9_$]*$`)
var visitorPattern = regexp.MustCompile(`^[A-Za-z0-9_%=+/-]+$`)
var syncIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,256}\|\|[A-Za-z0-9_-]{0,256}$`)

// Parse literals only. No eval, function calls, getters or prototype setters.
// YouTube's challenge may use single quotes, hex escapes and trailing commas.
type literalParser struct {
	s        string
	i, depth int
}

func (p *literalParser) space() {
	for p.i < len(p.s) && strings.ContainsRune(" \r\n\t", rune(p.s[p.i])) {
		p.i++
	}
}
func (p *literalParser) value() (any, error) {
	p.space()
	p.depth++
	defer func() { p.depth-- }()
	if p.depth > 64 || p.i >= len(p.s) {
		return nil, errors.New("config_invalid")
	}
	switch p.s[p.i] {
	case '\'', '"':
		return p.quoted()
	case '{':
		p.i++
		m := map[string]any{}
		p.space()
		for p.i < len(p.s) && p.s[p.i] != '}' {
			var key string
			var err error
			if p.s[p.i] == '\'' || p.s[p.i] == '"' {
				key, err = p.quoted()
			} else {
				start := p.i
				for p.i < len(p.s) && strings.ContainsRune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_$", rune(p.s[p.i])) {
					p.i++
				}
				key = p.s[start:p.i]
			}
			if err != nil || key == "" || key == "__proto__" {
				return nil, errors.New("config_invalid")
			}
			p.space()
			if p.i >= len(p.s) || p.s[p.i] != ':' {
				return nil, errors.New("config_invalid")
			}
			p.i++
			v, err := p.value()
			if err != nil {
				return nil, err
			}
			m[key] = v
			p.space()
			if p.i < len(p.s) && p.s[p.i] == ',' {
				p.i++
				p.space()
				continue
			}
			break
		}
		if p.i >= len(p.s) || p.s[p.i] != '}' {
			return nil, errors.New("config_invalid")
		}
		p.i++
		return m, nil
	case '[':
		p.i++
		a := []any{}
		p.space()
		for p.i < len(p.s) && p.s[p.i] != ']' {
			v, err := p.value()
			if err != nil {
				return nil, err
			}
			a = append(a, v)
			p.space()
			if p.i < len(p.s) && p.s[p.i] == ',' {
				p.i++
				p.space()
				continue
			}
			break
		}
		if p.i >= len(p.s) || p.s[p.i] != ']' {
			return nil, errors.New("config_invalid")
		}
		p.i++
		return a, nil
	default:
		start := p.i
		for p.i < len(p.s) && !strings.ContainsRune(",]} \t\r\n", rune(p.s[p.i])) {
			p.i++
		}
		var v any
		if json.Unmarshal([]byte(p.s[start:p.i]), &v) != nil {
			return nil, errors.New("config_invalid")
		}
		return v, nil
	}
}
func (p *literalParser) quoted() (string, error) {
	quote := p.s[p.i]
	p.i++
	var b strings.Builder
	for p.i < len(p.s) {
		c := p.s[p.i]
		p.i++
		if c == quote {
			return b.String(), nil
		}
		if c < ' ' {
			return "", errors.New("config_invalid")
		}
		if c != '\\' {
			b.WriteByte(c)
			continue
		}
		if p.i >= len(p.s) {
			break
		}
		e := p.s[p.i]
		p.i++
		switch e {
		case '\\', '\'', '"', '/':
			b.WriteByte(e)
		case 'n':
			b.WriteByte('\n')
		case 'r':
			b.WriteByte('\r')
		case 't':
			b.WriteByte('\t')
		case 'b':
			b.WriteByte('\b')
		case 'f':
			b.WriteByte('\f')
		case 'x', 'u':
			n := 2
			if e == 'u' {
				n = 4
			}
			if p.i+n > len(p.s) {
				return "", errors.New("config_invalid")
			}
			v, err := strconv.ParseUint(p.s[p.i:p.i+n], 16, 32)
			if err != nil {
				return "", errors.New("config_invalid")
			}
			p.i += n
			if v >= 0xD800 && v <= 0xDBFF {
				if p.i+6 > len(p.s) || p.s[p.i:p.i+2] != `\u` {
					return "", errors.New("config_invalid")
				}
				low, err := strconv.ParseUint(p.s[p.i+2:p.i+6], 16, 32)
				if err != nil || low < 0xDC00 || low > 0xDFFF {
					return "", errors.New("config_invalid")
				}
				p.i += 6
				v = uint64(utf16.DecodeRune(rune(v), rune(low)))
			} else if v >= 0xDC00 && v <= 0xDFFF {
				return "", errors.New("config_invalid")
			}
			b.WriteRune(rune(v))
		default:
			return "", errors.New("config_invalid")
		}
	}
	return "", errors.New("config_invalid")
}
func objectCalls(body []byte, pattern *regexp.Regexp, loose bool) ([]map[string]any, error) {
	if len(body) > maxResponse || !utf8.Valid(body) {
		return nil, errors.New("config_invalid")
	}
	s := string(body)
	out := []map[string]any{}
	for _, loc := range pattern.FindAllStringIndex(s, -1) {
		if loc[1] >= len(s) || s[loc[1]] != '{' {
			continue
		}
		p := literalParser{s: s, i: loc[1]}
		v, err := p.value()
		if err != nil {
			continue
		}
		p.space()
		if p.i >= len(s) || s[p.i] != ')' {
			continue
		}
		if !loose {
			var exact map[string]any
			if json.Unmarshal([]byte(s[loc[1]:p.i]), &exact) != nil {
				continue
			}
			v = exact
		}
		m, ok := v.(map[string]any)
		if ok {
			// BgUtils's loose literal contract decodes JSON strings at the
			// top level; the real ytAtN payload commonly puts R in a string.
			if loose {
				for key, item := range m {
					if s, ok := item.(string); ok {
						trim := strings.TrimSpace(s)
						if strings.HasPrefix(trim, "{") || strings.HasPrefix(trim, "[") {
							var decoded any
							if json.Unmarshal([]byte(trim), &decoded) == nil {
								m[key] = decoded
							}
						}
					}
				}
			}
			out = append(out, m)
		}
	}
	if len(out) == 0 {
		return nil, errors.New("config_missing")
	}
	return out, nil
}
func pageConfig(body []byte, anonymous bool) (map[string]any, error) {
	calls, err := objectCalls(body, configCalls, false)
	if err != nil {
		return nil, err
	}
	out := map[string]any{}
	for _, m := range calls {
		if anonymous && m["LOGGED_IN"] == true {
			return nil, errors.New("unexpected_authentication")
		}
		for k, v := range m {
			out[k] = v
		}
	}
	return out, nil
}
func parseBinding(body []byte, s Session) (proofBinding, error) {
	config, err := pageConfig(body, !s.LoggedIn)
	if err != nil {
		return proofBinding{}, err
	}
	visitor := textAt(config, "VISITOR_DATA")
	if visitor == "" {
		visitor = textAt(config, "INNERTUBE_CONTEXT", "client", "visitorData")
	}
	if len(visitor) > 4096 || !visitorPattern.MatchString(visitor) {
		return proofBinding{}, errors.New("visitor_missing")
	}
	b := proofBinding{Visitor: visitor, Binding: visitor}
	if s.LoggedIn {
		index := config["SESSION_INDEX"]
		if config["LOGGED_IN"] != true || index != float64(s.AccountIndex) && index != strconv.Itoa(s.AccountIndex) {
			return b, errors.New("account_mismatch")
		}
		syncID := textAt(config, "DATASYNC_ID")
		if !syncIDPattern.MatchString(syncID) {
			return b, errors.New("binding_missing")
		}
		parts := strings.Split(syncID, "||")
		if s.OnBehalfOfUser != "" && (parts[1] == "" || parts[0] != s.OnBehalfOfUser) || s.OnBehalfOfUser == "" && parts[1] != "" {
			return b, errors.New("brand_mismatch")
		}
		b.Binding = syncID
	}
	if raw := config["WEB_PLAYER_CONTEXT_CONFIGS"]; raw != nil {
		players, ok := raw.(map[string]any)
		if !ok {
			return b, errors.New("experiment_invalid")
		}
		for _, player := range players {
			flags := textAt(player, "serializedExperimentFlags")
			q, e := url.ParseQuery(flags)
			if e != nil {
				return b, errors.New("experiment_invalid")
			}
			values := q["html5_generate_content_po_token"]
			for _, v := range values {
				if v != "true" && v != "false" {
					return b, errors.New("experiment_invalid")
				}
			}
			if len(values) > 0 && values[len(values)-1] == "true" {
				b.VideoBound = true
			}
		}
	}
	return b, nil
}

type proofPage struct {
	Config      map[string]any `json:"config"`
	Program     string         `json:"program"`
	GlobalName  string         `json:"globalName"`
	Script      string         `json:"script"`
	interpreter string
}

func parseProofPage(body []byte) (proofPage, error) {
	config, err := pageConfig(body, true)
	if err != nil {
		return proofPage{}, err
	}
	calls, err := objectCalls(body, challengeCalls, true)
	if err != nil {
		return proofPage{}, err
	}
	for _, m := range calls {
		challenge := valueAt(m, "R", "bgChallenge")
		if s, ok := challenge.(string); ok {
			var v map[string]any
			if json.Unmarshal([]byte(s), &v) == nil {
				challenge = v
			}
		}
		if challenge == nil {
			continue
		}
		p := proofPage{Config: config, Program: textAt(challenge, "program"), GlobalName: textAt(challenge, "globalName")}
		if len(p.Program) == 0 || len(p.Program) > 8<<20 || len(p.GlobalName) > 128 || !globalNamePattern.MatchString(p.GlobalName) || p.GlobalName == "constructor" || p.GlobalName == "prototype" || p.GlobalName == "__proto__" {
			return p, errors.New("config_invalid")
		}
		raw := textAt(challenge, "interpreterUrl", "privateDoNotAccessOrElseTrustedResourceUrlWrappedValue")
		base, _ := url.Parse("https://www.youtube.com/")
		u, err := base.Parse(raw)
		if err != nil || raw == "" || len(raw) > 8192 || u.Host != "www.google.com" || !strings.HasPrefix(u.Path, "/js/th/") || !allowedRequest(u, "GET") {
			return p, errors.New("interpreter_route")
		}
		p.interpreter = u.String()
		return p, nil
	}
	return proofPage{}, errors.New("config_invalid")
}

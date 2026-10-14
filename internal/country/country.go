package country

import (
	"regexp"
	"sort"
	"strings"
)

// Matcher resolves a node name to a country/region code.
type Matcher struct {
	aliases map[string][]string
	codes   []string
}

// New builds a matcher from the alias table.
func New(aliases map[string][]string) *Matcher {
	codes := make([]string, 0, len(aliases))
	for k := range aliases {
		codes = append(codes, k)
	}
	sort.Strings(codes)
	return &Matcher{aliases: aliases, codes: codes}
}

const flagA = 0x1F1E6

// flagCountry decodes a leading/trailing regional-indicator flag emoji to an
// ISO 3166 alpha-2 lower-case code ("" when absent).
func flagCountry(name string) string {
	var codes []rune
	for _, ch := range name {
		o := int(ch)
		if o >= flagA && o <= flagA+25 {
			codes = append(codes, rune('A'+o-flagA))
		}
	}
	if len(codes) >= 2 {
		return strings.ToLower(string(codes[0]) + string(codes[1]))
	}
	return ""
}

var (
	normRe  = regexp.MustCompile(`[^a-z0-9\x{4e00}-\x{9fff}]`)
	tokenRe = regexp.MustCompile(`[^a-z0-9]+`)
)

func norm(s string) string {
	return normRe.ReplaceAllString(strings.ToLower(s), "")
}

func hasCJK(s string) bool {
	for _, r := range s {
		if r >= 0x4e00 && r <= 0x9fff {
			return true
		}
	}
	return false
}

var infoMarkers = []string{
	"剩余流量", "距离下次", "套餐到期", "到期", "官网", "流量", "重置",
	"订阅", "网址", "机场", "客服", "telegram", "http://", "https://",
}

// IsInfoNode reports whether name is an informational placeholder row.
func IsInfoNode(name string) bool {
	if name == "" {
		return true
	}
	low := strings.ToLower(name)
	for _, m := range infoMarkers {
		if strings.Contains(low, m) || strings.Contains(name, m) {
			return true
		}
	}
	return false
}

// CountryOf maps a node name to a country code ("" when unknown).
func (m *Matcher) CountryOf(name string) string {
	if name == "" {
		return ""
	}
	if f := flagCountry(name); f != "" {
		key := f
		if f == "gb" {
			key = "uk"
		}
		if _, ok := m.aliases[key]; ok {
			return key
		}
		return ""
	}
	n := norm(name)
	tokens := map[string]bool{}
	for _, t := range tokenRe.Split(strings.ToLower(name), -1) {
		if t != "" {
			tokens[t] = true
		}
	}
	for _, code := range m.codes {
		for _, kw := range m.aliases[code] {
			k := strings.ToLower(strings.TrimSpace(kw))
			if k == "" {
				continue
			}
			if hasCJK(k) {
				if strings.Contains(name, kw) || strings.Contains(name, k) {
					return code
				}
				continue
			}
			kn := norm(k)
			if kn == "" {
				continue
			}
			if len([]rune(kn)) >= 4 {
				if strings.Contains(n, kn) {
					return code
				}
			} else if tokens[kn] {
				return code
			}
		}
	}
	return ""
}

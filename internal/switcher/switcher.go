package switcher

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"verping/internal/api"
	"verping/internal/config"
	"verping/internal/country"
	"verping/internal/logger"
	"verping/internal/notify"
)

func getHistoryDelay(p api.Proxy) int {
	for i := len(p.History) - 1; i >= 0; i-- {
		if p.History[i].Delay > 0 {
			return p.History[i].Delay
		}
	}
	return 0
}

// RunOnce performs a single inspection/switch cycle using cfg.
func RunOnce(cfg config.Config) {
	threshold := cfg.LatencyThresholdMs
	group := cfg.TargetGroup
	httpTimeout := time.Duration(cfg.HTTPTimeoutMs) * time.Millisecond

	cli := api.New(cfg.ControllerURL, cfg.Secret, httpTimeout)
	m := country.New(cfg.CountryAliases)

	logger.Info("run once: check start")
	pr, err := cli.GetProxies()
	if err != nil {
		logger.Error("get proxies fail: " + err.Error())
		return
	}
	gp, ok := pr.Proxies[group]
	if !ok {
		logger.Warn("target group not found: " + group)
		return
	}
	cur := gp.Now
	if cur == "" {
		logger.Warn("current node empty")
		return
	}

	activeDelay := func(name string) int {
		return cli.DelayTest(name, cfg.TestURL, cfg.DelayTimeoutMs)
	}

	curDelay := getHistoryDelay(gp)
	if curDelay == 0 {
		if p, ok := pr.Proxies[cur]; ok {
			curDelay = getHistoryDelay(p)
		}
	}
	if curDelay == 0 {
		curDelay = activeDelay(cur)
		logger.Infof("current delay unknown, active test -> %d", curDelay)
	}
	logger.Infof("current node=%s delay=%d country=%s group=%s", cur, curDelay, m.CountryOf(cur), group)

	if curDelay > 0 && curDelay <= threshold {
		logger.Info("current <= threshold, skip")
		return
	}

	// Candidate names: drop duplicates and informational placeholder rows.
	var names []string
	seen := map[string]bool{}
	for _, name := range gp.All {
		if seen[name] || country.IsInfoNode(name) {
			continue
		}
		seen[name] = true
		names = append(names, name)
	}

	pool := map[string]int{}
	withHist := 0
	for _, n := range names {
		d := 0
		if p, ok := pr.Proxies[n]; ok {
			d = getHistoryDelay(p)
		}
		pool[n] = d
		if d > 0 {
			withHist++
		}
	}
	logger.Infof("candidates total=%d with-history=%d", len(names), withHist)

	maxConc := cfg.MaxConcurrentDelayTests
	if maxConc < 1 {
		maxConc = 4
	}
	testMany := func(cands []string) map[string]int {
		res := map[string]int{}
		var mu sync.Mutex
		var wg sync.WaitGroup
		sem := make(chan struct{}, maxConc)
		for _, n := range cands {
			wg.Add(1)
			go func(name string) {
				defer wg.Done()
				sem <- struct{}{}
				d := activeDelay(name)
				<-sem
				mu.Lock()
				res[name] = d
				mu.Unlock()
			}(n)
		}
		wg.Wait()
		return res
	}

	pick := func(cands []string) (string, int, bool) {
		type pair struct {
			name  string
			delay int
		}
		var usable []pair
		for _, n := range cands {
			d := pool[n]
			if d > 0 && d <= threshold && n != cur {
				usable = append(usable, pair{n, d})
			}
		}
		sort.Slice(usable, func(i, j int) bool { return usable[i].delay < usable[j].delay })
		if len(usable) == 0 {
			return "", 0, false
		}
		return usable[0].name, usable[0].delay, true
	}

	fillMissing := func(cands []string) {
		var need []string
		for _, n := range cands {
			if pool[n] == 0 {
				need = append(need, n)
			}
		}
		if len(need) > 0 {
			for n, d := range testMany(need) {
				if d > 0 {
					pool[n] = d
				}
			}
		}
	}

	sameCountryCands := make([]string, 0, len(names))
	if cfg.PreferSameCountry {
		cc := m.CountryOf(cur)
		if cc != "" {
			for _, n := range names {
				if m.CountryOf(n) == cc {
					sameCountryCands = append(sameCountryCands, n)
				}
			}
			fillMissing(sameCountryCands)
			if name, d, ok := pick(sameCountryCands); ok {
				if err := cli.PutSelect(group, name); err != nil {
					logger.Error("switch fail " + name + ": " + err.Error())
				} else {
					logger.Infof("switched to same-country < threshold: %s delay=%d country=%s", name, d, cc)
				}
				return
			}
			logger.Infof("no same-country < threshold (country=%s, tested=%d)", cc, len(sameCountryCands))
		} else {
			logger.Info("current country unknown, skip same-country stage")
		}
	}

	fillMissing(names)
	if name, d, ok := pick(names); ok {
		if err := cli.PutSelect(group, name); err != nil {
			logger.Error("switch fail " + name + ": " + err.Error())
		} else {
			logger.Infof("switched to global lowest < threshold: %s delay=%d", name, d)
		}
		return
	}

	if len(cfg.FallbackProfiles) > 0 {
		logger.Warn("all nodes > threshold, fallback_profiles configured but this build skips auto profile switch")
	}

	if len(names) == 0 {
		logger.Warn("no real candidate nodes found (all filtered), skip close")
		return
	}
	if cfg.TestMode {
		logger.Warn("[TEST_MODE] would call PATCH /configs {mode:off} and skip system popup")
		return
	}
	if err := cli.PatchModeOff(); err != nil {
		logger.Error("patch mode off fail: " + err.Error())
		return
	}
	logger.Warn("system proxy closed: all nodes > threshold")
	notify.Popup("Clash Verge 自动切换",
		fmt.Sprintf("系统代理已关闭：当前所有可用节点延时均 > %dms，暂无符合要求的低延时节点。", threshold))
}

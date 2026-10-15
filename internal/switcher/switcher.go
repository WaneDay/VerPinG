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

// getHistoryDelay returns the most recent usable delay sample, treating any
// sample older than maxAge as expired (returns 0 so the caller re-tests the
// node live). A maxAge <= 0 disables the age check; samples whose timestamp
// cannot be parsed are accepted as-is for backward compatibility.
func getHistoryDelay(p api.Proxy, maxAge time.Duration) int {
	now := time.Now()
	for i := len(p.History) - 1; i >= 0; i-- {
		h := p.History[i]
		if h.Delay <= 0 {
			continue
		}
		if maxAge > 0 {
			if t, err := time.Parse(time.RFC3339Nano, h.Time); err == nil && now.Sub(t) > maxAge {
				return 0
			}
		}
		return h.Delay
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

	// History samples older than two check intervals are treated as expired
	// ("unknown"), so the pool effectively resets every few cycles: a
	// long-stale sample can never pose as a fresh delay. Floor of 5 minutes
	// keeps the fast path usable on short intervals.
	maxAge := 2 * time.Duration(cfg.CheckIntervalMinutes) * time.Minute
	if maxAge < 5*time.Minute {
		maxAge = 5 * time.Minute
	}

	// The current node is always freshly tested ("refresh, then judge"),
	// so the decision matches what a dashboard manual test shows.
	// History is only a fallback when the live test fails.
	curDelay := activeDelay(cur)
	curSrc := "fresh"
	if curDelay == 0 {
		curDelay = getHistoryDelay(gp, maxAge)
		if curDelay == 0 {
			if p, ok := pr.Proxies[cur]; ok {
				curDelay = getHistoryDelay(p, maxAge)
			}
		}
		curSrc = "history"
	}
	if curDelay == 0 {
		logger.Warn("current delay unknown (live test + history both empty): " + cur)
	}
	logger.Infof("current node=%s delay=%d(%s) country=%s group=%s", cur, curDelay, curSrc, m.CountryOf(cur), group)

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
	if len(names) == 0 {
		logger.Warn("no real candidate nodes found (all filtered), skip")
		return
	}

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

	// Seed the pool from history; fresh tests below overwrite per node.
	// Samples past the TTL count as expired and are simply left empty so
	// they get re-tested (fillMissing) instead of trusted.
	pool := map[string]int{}
	withHist, expired := 0, 0
	for _, n := range names {
		d := 0
		if p, ok := pr.Proxies[n]; ok {
			d = getHistoryDelay(p, maxAge)
			if d == 0 && len(p.History) > 0 {
				expired++
			}
		}
		pool[n] = d
		if d > 0 {
			withHist++
		}
	}
	logger.Infof("candidates total=%d with-history=%d expired=%d", len(names), withHist, expired)

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

	doSelect := func(stage, name string, d int) {
		if err := cli.PutSelect(group, name); err != nil {
			logger.Error("switch fail " + name + ": " + err.Error())
		} else {
			logger.Infof("switched to %s: %s delay=%d", stage, name, d)
		}
	}

	// Fast path ("instant switch"): the panel keeps per-node delay history, so
	// when a node is already known to be under threshold we re-verify just
	// that one node live and switch, instead of sweeping the whole group. It
	// is still a real live check, so stale history cannot produce a bad
	// switch. Candidates are tried same-country first, then global (matching
	// the preference below); if neither verifies we fall through to the
	// thorough stages.
	cc := ""
	if cfg.PreferSameCountry {
		cc = m.CountryOf(cur)
	}
	fastProbe := func(cands []string) bool {
		name, _, ok := pick(cands)
		if !ok {
			return false
		}
		vd := activeDelay(name)
		if vd > 0 && vd <= threshold {
			doSelect("fast (history), verified", name, vd)
			return true
		}
		pool[name] = vd
		return false
	}
	if cc != "" {
		var sameCands []string
		for _, n := range names {
			if m.CountryOf(n) == cc {
				sameCands = append(sameCands, n)
			}
		}
		if fastProbe(sameCands) {
			return
		}
	}
	if fastProbe(names) {
		return
	}

	// Stage 1: same-country nodes, freshly tested (small set, cheap).
	if cfg.PreferSameCountry {
		cc := m.CountryOf(cur)
		if cc == "" {
			logger.Info("current country unknown, skip same-country stage")
		} else {
			var same []string
			for _, n := range names {
				if m.CountryOf(n) == cc {
					same = append(same, n)
				}
			}
			for n, d := range testMany(same) {
				if d > 0 {
					pool[n] = d
				}
			}
			if name, d, ok := pick(same); ok {
				doSelect("same-country < threshold", name, d)
				return
			}
			logger.Infof("no same-country < threshold (country=%s, tested=%d)", cc, len(same))
		}
	}

	// Stage 2: whole group by history, but verify the pick with one live
	// test before switching (up to 3 attempts) so stale history can neither
	// cause a bad switch nor hide behind an outdated number.
	fillMissing(names)
	for tried := 0; tried < 3; tried++ {
		name, d, ok := pick(names)
		if !ok {
			break
		}
		vd := activeDelay(name)
		if vd > 0 && vd <= threshold {
			doSelect("global lowest < threshold", name, vd)
			return
		}
		logger.Infof("verify failed, skip candidate: %s history=%d fresh=%d", name, d, vd)
		pool[name] = vd
	}

	// Fallback: everything is above threshold — switch to the currently
	// lowest-delay node and pop a reminder, then keep the timed loop going.
	fillMissing(names)
	best, bd := "", 0
	for _, n := range names {
		if d := pool[n]; d > 0 && (bd == 0 || d < bd) {
			best, bd = n, d
		}
	}
	if best == "" {
		logger.Warn("all candidates delay unknown, skip")
		return
	}
	// After the round-robin retests the current node may already be back
	// inside the threshold and simply be the group's lowest. That is a
	// healthy state, not an all-bad stretch, so no switch and no popup.
	if best == cur && bd <= threshold {
		logger.Infof("current node already lowest and within threshold: %s delay=%d (threshold=%d)", cur, bd, threshold)
		return
	}
	// NOTE: the fallback switch runs even in test_mode (same class of action
	// as stage 1/2, which also run in test_mode); test_mode only suppresses
	// the popup, so a bad stretch stays visible yet silent.
	if best == cur {
		logger.Warnf("already on lowest-delay node: %s delay=%d > threshold=%d", cur, bd, threshold)
	} else if err := cli.PutSelect(group, best); err != nil {
		logger.Error("fallback switch fail " + best + ": " + err.Error())
		return
	} else if cfg.TestMode {
		logger.Warnf("[TEST_MODE] switched to lowest (no popup): %s delay=%d (threshold=%d)", best, bd, threshold)
	} else {
		logger.Warnf("fallback: all > threshold, switched to lowest: %s delay=%d (threshold=%d)", best, bd, threshold)
	}
	if !cfg.TestMode {
		popupFallbackOnce(group, best, bd, threshold)
	}
}

// lastPopupKey suppresses repeat popups while stuck on the same fallback
// target, so a long all-bad stretch reminds once instead of every cycle.
var lastPopupKey string

func popupFallbackOnce(group, best string, bd, threshold int) {
	key := group + "|" + best
	if key == lastPopupKey {
		logger.Info("fallback popup suppressed (unchanged): " + best)
		return
	}
	lastPopupKey = key
	go notify.Popup("VerPinG",
		fmt.Sprintf("当前所有节点延时均超过阈值 %dms。\r\n已切换到相对最低延时节点：%s（%dms）。\r\n将继续定时巡检，发现符合要求的节点会自动切换。", threshold, best, bd))
}

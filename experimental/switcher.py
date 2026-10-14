import json, os, re, time, urllib.request, urllib.parse
from concurrent.futures import ThreadPoolExecutor
from datetime import datetime

BASE = os.path.dirname(os.path.abspath(__file__))
CFG_PATH = os.path.join(BASE, "config.json")
LOG_DIR = os.path.join(BASE, "logs")
LOG_PATH = os.path.join(LOG_DIR, "app.log")
os.makedirs(LOG_DIR, exist_ok=True)


def log(msg, level="INFO"):
    line = f"{datetime.now().isoformat(timespec='seconds')} [{level}] {msg}"
    try:
        with open(LOG_PATH, "a", encoding="utf-8") as f:
            f.write(line + "\n")
    except Exception:
        pass


def load_cfg():
    with open(CFG_PATH, encoding="utf-8") as f:
        return json.load(f)


def _request(method, url, secret, data=None, timeout=5):
    headers = {"User-Agent": "verge-latency-switcher-py/0.2"}
    if secret:
        headers["Authorization"] = "Bearer " + secret
    if data is not None:
        headers["Content-Type"] = "application/json"
    req = urllib.request.Request(url, data=data, method=method, headers=headers)
    with urllib.request.urlopen(req, timeout=timeout) as r:
        raw = r.read().decode("utf-8", "ignore")
        try:
            return r.status, json.loads(raw) if raw else {}
        except Exception:
            return r.status, raw


def api_get(base, path, secret, timeout=5):
    _, body = _request("GET", base.rstrip("/") + path, secret, timeout=timeout)
    return body


def api_put_select(base, group, node, secret, timeout=5):
    data = json.dumps({"name": node}).encode()
    status, _ = _request("PUT", base.rstrip("/") + "/proxies/" + urllib.parse.quote(group, safe=""), secret, data, timeout)
    return status


def api_patch_mode_off(base, secret, timeout=5):
    data = json.dumps({"mode": "off"}).encode()
    status, _ = _request("PATCH", base.rstrip("/") + "/configs", secret, data, timeout)
    return status


def api_delay_test(base, node, secret, test_url, delay_timeout_ms, timeout=5):
    q = urllib.parse.urlencode({"url": test_url, "timeout": str(int(delay_timeout_ms))})
    path = "/proxies/" + urllib.parse.quote(node, safe="") + "/delay?" + q
    try:
        _, body = _request("GET", base.rstrip("/") + path, secret, timeout=timeout)
        if isinstance(body, dict):
            return int(body.get("delay", 0) or 0)
    except Exception:
        pass
    return 0


# ---------------- country matching ----------------
_FLAG_A = 0x1F1E6
ISO_MAP = {"gb": "uk"}


def _flag_country(name):
    codes = []
    for ch in name:
        o = ord(ch)
        if _FLAG_A <= o <= _FLAG_A + 25:
            codes.append(chr(o - _FLAG_A + ord("A")))
    if len(codes) >= 2:
        return (codes[0] + codes[1]).lower()
    return ""


def _norm(s):
    return re.sub(r"[^a-z0-9\u4e00-\u9fff]", "", s.lower())


INFO_MARKERS = ["剩余流量", "距离下次", "套餐到期", "到期", "官网", "流量", "重置",
                "订阅", "网址", "机场", "客服", "telegram", "http://", "https://"]


def is_info_node(name):
    if not name:
        return True
    low = name.lower()
    for m in INFO_MARKERS:
        if m in low or m in name:
            return True
    return False


def country_of(name, aliases):
    if not name:
        return ""
    flag = _flag_country(name)
    if flag:
        key = ISO_MAP.get(flag, flag)
        return key if key in aliases else ""
    norm = _norm(name)
    tokens = set(t for t in re.split(r"[^a-z0-9]+", name.lower()) if t)
    for code, kws in aliases.items():
        for kw in kws:
            k = (kw or "").lower().strip()
            if not k:
                continue
            if re.search(r"[\u4e00-\u9fff]", k):
                if kw in name or k in name:
                    return code
                continue
            kn = _norm(k)
            if not kn:
                continue
            if len(kn) >= 4:
                if kn in norm:
                    return code
            elif kn in tokens:
                return code
    return ""


def get_history_delay(proxy):
    hist = (proxy or {}).get("history") or []
    for h in reversed(hist):
        d = h.get("delay", 0) or 0
        if d and d > 0:
            return d
    return 0


# ---------------- core ----------------
def run_once(cfg):
    base = cfg.get("controller_url", "http://127.0.0.1:9090")
    secret = cfg.get("secret", "")
    group = cfg.get("target_group", "自动选择")
    threshold = int(cfg.get("latency_threshold_ms", 220))
    prefer_same = bool(cfg.get("prefer_same_country", True))
    aliases = cfg.get("country_aliases", {})
    test_mode = bool(cfg.get("test_mode", True))
    http_timeout = int(cfg.get("http_timeout_ms", 5000)) / 1000.0
    test_url = cfg.get("test_url", "https://cp.cloudflare.com/generate_204")
    delay_timeout_ms = int(cfg.get("delay_timeout_ms", 5000))
    max_conc = max(1, int(cfg.get("max_concurrent_delay_tests", 4)))

    log("run once: check start")
    try:
        data = api_get(base, "/proxies", secret, http_timeout)
    except Exception as e:
        log(f"get proxies fail: {e}", "ERROR")
        return
    proxies = data.get("proxies", {}) if isinstance(data, dict) else {}
    if group not in proxies:
        log(f"target group not found: {group} (available: {list(proxies.keys())[:10]})", "WARN")
        return

    gp = proxies[group]
    cur = gp.get("now", "")
    if not cur:
        log("current node empty", "WARN")
        return

    def active_delay(name):
        return api_delay_test(base, name, secret, test_url, delay_timeout_ms, http_timeout)

    cur_delay = get_history_delay(gp)
    if cur_delay == 0 and cur in proxies:
        cur_delay = get_history_delay(proxies[cur])
    if cur_delay == 0:
        cur_delay = active_delay(cur)
        log(f"current delay unknown, active test -> {cur_delay}")
    log(f"current node={cur} delay={cur_delay} country={country_of(cur, aliases)} group={group}")

    if cur_delay and 0 < cur_delay <= threshold:
        log("current <= threshold, skip")
        return

    names = []
    seen = set()
    for name in (gp.get("all") or []):
        if name in seen or is_info_node(name):
            continue
        seen.add(name)
        names.append(name)

    def delay_of(name):
        d = get_history_delay(proxies.get(name, {}))
        return d

    pool = {n: delay_of(n) for n in names}
    log(f"candidates total={len(names)} with-history={sum(1 for v in pool.values() if v > 0)}")

    def test_many(cands):
        result = {}
        with ThreadPoolExecutor(max_workers=max_conc) as ex:
            futs = {ex.submit(active_delay, n): n for n in cands}
            for f in futs:
                n = futs[f]
                try:
                    result[n] = f.result()
                except Exception:
                    result[n] = 0
        return result

    def pick(cands):
        usable = []
        for n in cands:
            d = pool.get(n, 0)
            if d and d > 0 and d <= threshold and n != cur:
                usable.append((n, d))
        usable.sort(key=lambda x: x[1])
        return usable[0] if usable else None

    # 1) same country (only active-test nodes that have no history)
    if prefer_same:
        cc = country_of(cur, aliases)
        if cc:
            same = [n for n in names if country_of(n, aliases) == cc]
            need = [n for n in same if pool.get(n, 0) == 0]
            if need:
                fresh = test_many(need)
                for n, d in fresh.items():
                    if d > 0:
                        pool[n] = d
            best = pick(same)
            if best:
                try:
                    api_put_select(base, group, best[0], secret, http_timeout)
                    log(f"switched to same-country < threshold: {best[0]} delay={best[1]} country={cc}")
                except Exception as e:
                    log(f"switch fail {best[0]}: {e}", "ERROR")
                return
            log(f"no same-country < threshold (country={cc}, tested={len(same)})")
        else:
            log("current country unknown, skip same-country stage")

    # 2) global: active-test only candidates without history
    need = [n for n in names if pool.get(n, 0) == 0]
    if need:
        fresh = test_many(need)
        for n, d in fresh.items():
            if d > 0:
                pool[n] = d
    best = pick(names)
    if best:
        try:
            api_put_select(base, group, best[0], secret, http_timeout)
            log(f"switched to global lowest < threshold: {best[0]} delay={best[1]}")
        except Exception as e:
            log(f"switch fail {best[0]}: {e}", "ERROR")
        return

    # 3) fallback profiles (not implemented in py version)
    fps = cfg.get("fallback_profiles") or []
    if fps:
        log(f"all nodes > threshold, fallback_profiles configured but py version skips auto profile switch: {fps}", "WARN")

    # 4) close proxy
    if not names:
        log("no real candidate nodes found (all filtered), skip close", "WARN")
        return
    if test_mode:
        log("[TEST_MODE] would call PATCH /configs {mode:off} and skip system popup", "WARN")
        return
    try:
        api_patch_mode_off(base, secret, http_timeout)
        log("system proxy closed: all nodes > threshold", "WARN")
    except Exception as e:
        log(f"patch mode off fail: {e}", "ERROR")
        return
    try:
        import ctypes
        ctypes.windll.user32.MessageBoxW(0, f"系统代理已关闭：当前所有可用节点延时均 > {threshold}ms，暂无符合要求的低延时节点。", "Clash Verge 自动切换", 0x30)
    except Exception as e:
        log(f"popup fail: {e}", "ERROR")


def main():
    cfg = load_cfg()
    interval = max(1, int(cfg.get("check_interval_minutes", 5)))
    log("first run (py version)")
    run_once(cfg)
    while True:
        time.sleep(interval * 60)
        try:
            cfg = load_cfg()
        except Exception as e:
            log(f"reload config fail: {e}", "ERROR")
        run_once(cfg)


if __name__ == "__main__":
    main()

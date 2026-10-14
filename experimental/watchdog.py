import os, sys, time, ctypes
from ctypes import wintypes

BASE = os.path.dirname(os.path.abspath(__file__))
LOG_DIR = os.path.join(BASE, 'logs')
os.makedirs(LOG_DIR, exist_ok=True)
LOG_PATH = os.path.join(LOG_DIR, 'app.log')
APP = os.path.join(BASE, 'app.py')
APP_PID = os.path.join(LOG_DIR, 'app.pid')
WD_PID = os.path.join(LOG_DIR, 'watchdog.pid')
MUTEX_WD = 'Local\\VergeLatencySwitcherWatchdog'
MUTEX_APP = 'Local\\VergeLatencySwitcherApp'
POLL_SECONDS = 5

k32 = ctypes.windll.kernel32
PROCESS_QUERY_LIMITED_INFORMATION = 0x1000
PROCESS_TERMINATE = 0x0001
SYNCHRONIZE = 0x00100000
STILL_ACTIVE = 259
CREATE_NO_WINDOW = 0x08000000


def log(msg):
    try:
        import datetime
        with open(LOG_PATH, 'a', encoding='utf-8') as f:
            f.write(f"{datetime.datetime.now().isoformat(timespec='seconds')} [WATCHDOG] {msg}\n")
    except Exception:
        pass


def pythonw():
    exe = sys.executable or ''
    cand = os.path.join(os.path.dirname(exe), 'pythonw.exe')
    return cand if os.path.exists(cand) else exe


def system_proxy_on():
    try:
        import winreg
        key = winreg.OpenKey(winreg.HKEY_CURRENT_USER,
                             r"Software\Microsoft\Windows\CurrentVersion\Internet Settings")
        try:
            enable, _ = winreg.QueryValueEx(key, "ProxyEnable")
        except FileNotFoundError:
            enable = 0
        try:
            server, _ = winreg.QueryValueEx(key, "ProxyServer")
        except FileNotFoundError:
            server = ""
        winreg.CloseKey(key)
        return bool(enable) and bool(str(server).strip())
    except Exception:
        return False


def pid_alive(pid):
    try:
        pid = int(pid)
    except Exception:
        return False
    h = k32.OpenProcess(PROCESS_QUERY_LIMITED_INFORMATION, False, pid)
    if not h:
        return False
    code = wintypes.DWORD()
    ok = k32.GetExitCodeProcess(h, ctypes.byref(code))
    k32.CloseHandle(h)
    return bool(ok) and code.value == STILL_ACTIVE


def read_pid(path):
    try:
        with open(path) as f:
            return int(f.read().strip())
    except Exception:
        return None


def app_running():
    h = k32.OpenMutexW(SYNCHRONIZE, False, MUTEX_APP)
    if h:
        k32.CloseHandle(h)
        return True
    pid = read_pid(APP_PID)
    return pid_alive(pid) if pid else False


def stop_app():
    pid = read_pid(APP_PID)
    if not pid and app_running():
        for _ in range(10):
            time.sleep(0.5)
            pid = read_pid(APP_PID)
            if pid:
                break
    if pid and pid_alive(pid):
        h = k32.OpenProcess(PROCESS_TERMINATE, False, pid)
        if h:
            k32.TerminateProcess(h, 0)
            k32.CloseHandle(h)
            log(f"system proxy OFF -> stopped app pid={pid}")
    try:
        os.remove(APP_PID)
    except Exception:
        pass


def start_app():
    import subprocess
    subprocess.Popen([pythonw(), APP], cwd=BASE, creationflags=CREATE_NO_WINDOW,
                     stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, close_fds=True)
    log("system proxy ON -> started app")


def main():
    k32.CreateMutexW(None, False, MUTEX_WD)
    if k32.GetLastError() == 183:
        return
    with open(WD_PID, 'w') as f:
        f.write(str(os.getpid()))
    log("watchdog started")
    last = None
    while True:
        cur = system_proxy_on()
        if cur:
            if last is not True and not app_running():
                start_app()
        else:
            if app_running():
                stop_app()
        if cur != last:
            log(f"system proxy -> {'ON' if cur else 'OFF'}")
            last = cur
        time.sleep(POLL_SECONDS)


if __name__ == '__main__':
    main()

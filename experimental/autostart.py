import os, sys, subprocess, ctypes

BASE = os.path.dirname(os.path.abspath(__file__))
WATCHDOG = os.path.join(BASE, 'watchdog.py')
WD_PID = os.path.join(BASE, 'logs', 'watchdog.pid')
RUN_KEY = r"Software\Microsoft\Windows\CurrentVersion\Run"
RUN_NAME = "VergeLatencySwitcher"
MUTEX_WD = 'Local\\VergeLatencySwitcherWatchdog'
CREATE_NO_WINDOW = 0x08000000
SYNCHRONIZE = 0x00100000
PROCESS_TERMINATE = 0x0001
STILL_ACTIVE = 259
PROCESS_QUERY_LIMITED_INFORMATION = 0x1000

k32 = ctypes.windll.kernel32


def pythonw():
    exe = sys.executable or ''
    cand = os.path.join(os.path.dirname(exe), 'pythonw.exe')
    return cand if os.path.exists(cand) else exe


def run_command():
    q = '"'
    return f'{q}{pythonw()}{q} {q}{WATCHDOG}{q}'


def is_enabled():
    try:
        import winreg
        key = winreg.OpenKey(winreg.HKEY_CURRENT_USER, RUN_KEY)
    except FileNotFoundError:
        return False
    try:
        winreg.QueryValueEx(key, RUN_NAME)
        return True
    except FileNotFoundError:
        return False
    finally:
        winreg.CloseKey(key)


def _read_pid():
    try:
        with open(WD_PID) as f:
            return int(f.read().strip())
    except Exception:
        return None


def _pid_alive(pid):
    try:
        pid = int(pid)
    except Exception:
        return False
    h = k32.OpenProcess(PROCESS_QUERY_LIMITED_INFORMATION, False, pid)
    if not h:
        return False
    code = ctypes.c_ulong()
    ok = k32.GetExitCodeProcess(h, ctypes.byref(code))
    k32.CloseHandle(h)
    return bool(ok) and code.value == STILL_ACTIVE


def watchdog_running():
    h = k32.OpenMutexW(SYNCHRONIZE, False, MUTEX_WD)
    if h:
        k32.CloseHandle(h)
        return True
    pid = _read_pid()
    return _pid_alive(pid) if pid else False


def start_watchdog():
    if watchdog_running():
        return
    subprocess.Popen([pythonw(), WATCHDOG], cwd=BASE, creationflags=CREATE_NO_WINDOW,
                     stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, close_fds=True)


def stop_watchdog():
    pid = _read_pid()
    if pid and _pid_alive(pid):
        h = k32.OpenProcess(PROCESS_TERMINATE, False, pid)
        if h:
            k32.TerminateProcess(h, 0)
            k32.CloseHandle(h)
    try:
        os.remove(WD_PID)
    except Exception:
        pass


def enable():
    import winreg
    key = winreg.CreateKey(winreg.HKEY_CURRENT_USER, RUN_KEY)
    try:
        winreg.SetValueEx(key, RUN_NAME, 0, winreg.REG_SZ, run_command())
    finally:
        winreg.CloseKey(key)
    start_watchdog()


def disable():
    try:
        import winreg
        key = winreg.OpenKey(winreg.HKEY_CURRENT_USER, RUN_KEY, 0, winreg.KEY_SET_VALUE)
    except FileNotFoundError:
        key = None
    if key:
        try:
            winreg.DeleteValue(key, RUN_NAME)
        except FileNotFoundError:
            pass
        finally:
            winreg.CloseKey(key)
    stop_watchdog()

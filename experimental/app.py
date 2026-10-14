import os, sys, threading, atexit, ctypes
from PIL import Image
import pystray
from pystray import Menu, MenuItem

BASE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, BASE)

import switcher
from ui import UI

ICON_PNG = os.path.join(BASE, 'icon.png')
ICON_ICO = os.path.join(BASE, 'icon.ico')
PID_PATH = os.path.join(BASE, 'logs', 'app.pid')
MUTEX_NAME = 'Local\\VergeLatencySwitcherApp'


def set_app_id():
    try:
        ctypes.windll.shell32.SetCurrentProcessExplicitAppUserModelID('verge.latency.switcher.1')
    except Exception:
        pass


def acquire_single_instance():
    handle = ctypes.windll.kernel32.CreateMutexW(None, False, MUTEX_NAME)
    if ctypes.windll.kernel32.GetLastError() == 183:
        return None
    return handle


def write_pid():
    try:
        with open(PID_PATH, 'w') as f:
            f.write(str(os.getpid()))
    except Exception:
        pass


def remove_pid():
    try:
        os.remove(PID_PATH)
    except Exception:
        pass


def load_image():
    for p in (ICON_ICO, ICON_PNG):
        if os.path.exists(p):
            try:
                return Image.open(p)
            except Exception:
                pass
    return Image.new('RGBA', (64, 64), (59, 130, 246, 255))


class App:
    def __init__(self, show=True):
        self.ui = UI(on_close=self.hide, start_hidden=not show)
        self.icon = pystray.Icon(
            'verge-latency-switcher',
            load_image(),
            'Verge 延时自动切换',
            menu=Menu(
                MenuItem('打开界面', self.show, default=True),
                MenuItem('隐藏界面', self.hide),
                Menu.SEPARATOR,
                MenuItem('退出', self.quit),
            ),
        )

    def start(self):
        threading.Thread(target=self._switcher_loop, daemon=True).start()
        threading.Thread(target=self._tray_loop, daemon=True).start()
        self.ui.mainloop()

    def _log(self, msg):
        try:
            import datetime
            with open(os.path.join(BASE, 'logs', 'app.log'), 'a', encoding='utf-8') as f:
                f.write(f"{datetime.datetime.now().isoformat(timespec='seconds')} [INFO] {msg}\n")
        except Exception:
            pass

    def _switcher_loop(self):
        try:
            switcher.main()
        except Exception as e:
            self._log(f"switcher loop stopped: {e}")

    def _tray_loop(self):
        self._log(f"tray thread started (backend ready)")
        try:
            self.icon.run()
            self._log("tray icon stopped")
        except Exception as e:
            self._log(f"tray icon failed: {e}")
            self._log("tray backend unavailable (pystray)")

    def show(self, *a):
        try:
            self.ui.after(0, self.ui.show_window)
        except Exception:
            pass

    def hide(self, *a):
        try:
            self.ui.after(0, self.ui.hide_window)
        except Exception:
            pass

    def quit(self, *a):
        try:
            self.icon.stop()
        except Exception:
            pass
        try:
            self.ui.after(0, self.ui.destroy)
        except Exception:
            pass


if __name__ == '__main__':
    set_app_id()
    if acquire_single_instance() is None:
        sys.exit(0)
    write_pid()
    atexit.register(remove_pid)
    App(show='--show' in sys.argv).start()

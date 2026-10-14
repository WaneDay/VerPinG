import os, json, tkinter as tk
from tkinter import ttk
import autostart

BASE = os.path.dirname(os.path.abspath(__file__))
CFG = os.path.join(BASE, 'config.json')
LOG = os.path.join(BASE, 'logs', 'app.log')
ICON = os.path.join(BASE, 'icon.ico')
os.makedirs(os.path.join(BASE, 'logs'), exist_ok=True)


def load_config():
    with open(CFG, encoding='utf-8') as f:
        return json.load(f)


def save_config(d):
    with open(CFG, 'w', encoding='utf-8') as f:
        json.dump(d, f, indent=2, ensure_ascii=False)


class UI(tk.Tk):
    def __init__(self, on_close=None, start_hidden=False):
        super().__init__()
        self.on_close = on_close
        self.title('Verge 延时自动切换')
        self.geometry('480x460')
        self.minsize(460, 430)
        try:
            self.iconbitmap(ICON)
        except Exception:
            pass
        self.cfg = load_config()

        main = ttk.Frame(self, padding=10)
        main.pack(fill=tk.BOTH, expand=True)

        box = ttk.LabelFrame(main, text='设置', padding=6)
        box.pack(fill=tk.X)

        grid = ttk.Frame(box)
        grid.pack(fill=tk.X)
        grid.columnconfigure(1, weight=1)

        def row(r, label, widget):
            ttk.Label(grid, text=label).grid(row=r, column=0, sticky='w', padx=2, pady=3)
            widget.grid(row=r, column=1, sticky='ew', padx=6, pady=3)
            return widget

        self.var_interval = tk.IntVar(value=self.cfg.get('check_interval_minutes', 5))
        self.var_latency = tk.IntVar(value=self.cfg.get('latency_threshold_ms', 220))
        self.var_group = tk.StringVar(value=self.cfg.get('target_group', '自动选择'))
        self.var_url = tk.StringVar(value=self.cfg.get('controller_url', 'http://127.0.0.1:9090'))
        self.var_secret = tk.StringVar(value=self.cfg.get('secret', ''))

        row(0, '定时时长(分钟)', ttk.Spinbox(grid, from_=1, to=120, textvariable=self.var_interval, width=8))
        row(1, '延时阈值(ms)', ttk.Spinbox(grid, from_=50, to=2000, textvariable=self.var_latency, width=8))
        row(2, '目标策略组', ttk.Entry(grid, textvariable=self.var_group))
        row(3, 'API 地址', ttk.Entry(grid, textvariable=self.var_url))
        row(4, '密钥', ttk.Entry(grid, textvariable=self.var_secret, show='*'))

        opt = ttk.Frame(box)
        opt.pack(fill=tk.X, pady=(6, 0))
        self.var_country = tk.BooleanVar(value=self.cfg.get('prefer_same_country', True))
        self.var_test = tk.BooleanVar(value=self.cfg.get('test_mode', True))
        ttk.Checkbutton(opt, text='优先同国家', variable=self.var_country).pack(side=tk.LEFT, padx=2)
        ttk.Checkbutton(opt, text='静默模式(不弹窗不关代理)', variable=self.var_test).pack(side=tk.LEFT, padx=6)

        opt2 = ttk.Frame(box)
        opt2.pack(fill=tk.X, pady=(4, 0))
        self.var_autostart = tk.BooleanVar(value=autostart.is_enabled())
        ttk.Checkbutton(opt2, text='开机自启(随系统代理开关)', variable=self.var_autostart,
                        command=self.toggle_autostart).pack(side=tk.LEFT, padx=2)

        bar = ttk.Frame(box)
        bar.pack(fill=tk.X, pady=(6, 0))
        ttk.Button(bar, text='保存设置', command=self.save).pack(side=tk.LEFT)
        ttk.Button(bar, text='重载配置', command=self.reload).pack(side=tk.LEFT, padx=6)
        self.status = ttk.Label(bar, text='')
        self.status.pack(side=tk.RIGHT)

        lbox = ttk.LabelFrame(main, text='运行日志', padding=6)
        lbox.pack(fill=tk.BOTH, expand=True, pady=(8, 0))
        self.log_txt = tk.Text(lbox, height=10, wrap='none', state='disabled')
        self.log_txt.pack(fill=tk.BOTH, expand=True)
        self.log_last = ''

        self.protocol('WM_DELETE_WINDOW', self._close_clicked)
        self.after(2000, self.refresh_log)
        if start_hidden:
            self.withdraw()

    def _close_clicked(self):
        if self.on_close:
            self.on_close()
        else:
            self.destroy()

    def show_window(self):
        self.deiconify()
        self.lift()
        try:
            self.attributes('-topmost', True)
            self.after(250, lambda: self.attributes('-topmost', False))
        except Exception:
            pass

    def hide_window(self):
        self.withdraw()

    def toggle_autostart(self):
        try:
            if self.var_autostart.get():
                autostart.enable()
                self.status.config(text='已启用开机自启 ✓')
            else:
                autostart.disable()
                self.status.config(text='已关闭开机自启 ✓')
        except Exception as e:
            self.status.config(text=f'自启操作失败: {e}')
            self.var_autostart.set(autostart.is_enabled())

    def save(self):
        try:
            self.cfg['check_interval_minutes'] = self.var_interval.get()
            self.cfg['latency_threshold_ms'] = self.var_latency.get()
            self.cfg['target_group'] = self.var_group.get().strip()
            self.cfg['controller_url'] = self.var_url.get().strip()
            self.cfg['secret'] = self.var_secret.get()
            self.cfg['prefer_same_country'] = self.var_country.get()
            self.cfg['test_mode'] = self.var_test.get()
            save_config(self.cfg)
            self.status.config(text='已保存 ✓')
        except Exception as e:
            self.status.config(text=f'保存失败: {e}')

    def reload(self):
        try:
            self.cfg = load_config()
            self.var_interval.set(self.cfg.get('check_interval_minutes', 5))
            self.var_latency.set(self.cfg.get('latency_threshold_ms', 220))
            self.var_group.set(self.cfg.get('target_group', '自动选择'))
            self.var_url.set(self.cfg.get('controller_url', 'http://127.0.0.1:9090'))
            self.var_secret.set(self.cfg.get('secret', ''))
            self.var_country.set(self.cfg.get('prefer_same_country', True))
            self.var_test.set(self.cfg.get('test_mode', True))
            self.var_autostart.set(autostart.is_enabled())
            self.status.config(text='已重载 ✓')
        except Exception as e:
            self.status.config(text=f'重载失败: {e}')

    def refresh_log(self):
        try:
            with open(LOG, encoding='utf-8', errors='ignore') as f:
                content = f.read()
            if content != self.log_last:
                self.log_txt.config(state='normal')
                self.log_txt.delete('1.0', tk.END)
                self.log_txt.insert('1.0', content[-20000:])
                self.log_txt.see(tk.END)
                self.log_txt.config(state='disabled')
                self.log_last = content
        except Exception:
            pass
        self.after(2000, self.refresh_log)


if __name__ == '__main__':
    UI().mainloop()

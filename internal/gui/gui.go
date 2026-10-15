package gui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
	"unsafe"

	"github.com/lxn/walk"
	"github.com/lxn/win"

	"verping/internal/agent"
	"verping/internal/api"
	"verping/internal/autostart"
	"verping/internal/config"
	"verping/internal/logger"
)

type guiState struct {
	dir      string
	ag       *agent.Agent
	cfg      config.Config
	loading  bool
	quitting bool
}

func loadIcon(exeDir string) walk.Image {
	candidates := []string{
		filepath.Join(exeDir, "icon.ico"),
		filepath.Join(exeDir, "resources", "icon.ico"),
	}
	for _, p := range candidates {
		if ic, err := walk.NewIconFromFile(p); err == nil {
			return ic
		}
	}
	return nil
}

// editAtBottom reports whether a log view is scrolled to the end,
// so auto-scroll never yanks the wheel away while reading history.
func editAtBottom(te *walk.TextEdit) bool {
	var si win.SCROLLINFO
	si.CbSize = uint32(unsafe.Sizeof(si))
	si.FMask = win.SIF_RANGE | win.SIF_PAGE | win.SIF_POS
	if !win.GetScrollInfo(te.Handle(), win.SB_VERT, &si) {
		return true
	}
	return int(si.NPos)+int(si.NPage) >= int(si.NMax)
}

// setLogText swaps the whole buffer with WM_SETTEXT only. It deliberately
// never uses AppendText/SetTextSelection: those send EM_SETSEL/EM_REPLACESEL,
// which scroll the caret into view and make Windows play the default beep.
// Scroll is restored with WM_VSCROLL / EM_LINESCROLL instead, both silent.
func setLogText(te *walk.TextEdit, txt string, follow bool) {
	hwnd := te.Handle()
	first := int32(0)
	if !follow {
		first = int32(win.SendMessage(hwnd, win.EM_GETFIRSTVISIBLELINE, 0, 0))
	}
	_ = te.SetText(txt)
	if follow {
		win.SendMessage(hwnd, win.WM_VSCROLL, win.SB_BOTTOM, 0)
		return
	}
	if delta := first - int32(win.SendMessage(hwnd, win.EM_GETFIRSTVISIBLELINE, 0, 0)); delta != 0 {
		win.SendMessage(hwnd, win.EM_LINESCROLL, 0, uintptr(int64(delta)))
	}
}

// Run builds and runs the native window + tray icon. It blocks until exit and
// must be called on the main goroutine.
func Run(dir, exeDir string, show bool, ag *agent.Agent) {
	runtime.LockOSThread()

	mw, err := walk.NewMainWindow()
	if err != nil {
		logger.Error("create window fail: " + err.Error())
		return
	}

	g := &guiState{dir: dir, ag: ag, cfg: ag.Load()}

	icon := loadIcon(exeDir)
	if icon != nil {
		_ = mw.SetIcon(icon)
	} else {
		logger.Warn("icon.ico not found next to exe, window/tray icons will be empty")
	}
	_ = mw.SetTitle("VerPinG")
	_ = mw.SetSize(walk.Size{Width: 520, Height: 560})

	// MainWindow must own a layout of its own, otherwise Show triggers a nil
	// layout panic. HBox + side spacers keeps the content visually centered.
	if err := mw.SetLayout(walk.NewHBoxLayout()); err != nil {
		logger.Error("set layout fail: " + err.Error())
		return
	}
	if _, err := walk.NewHSpacer(mw); err != nil {
		logger.Error("spacer fail: " + err.Error())
		return
	}
	center, err := walk.NewComposite(mw)
	if err != nil {
		logger.Error("center fail: " + err.Error())
		return
	}
	_ = center.SetLayout(walk.NewVBoxLayout())
	_ = center.SetMinMaxSize(walk.Size{Width: 480, Height: 0}, walk.Size{Width: 480, Height: 0})
	if _, err := walk.NewHSpacer(mw); err != nil {
		logger.Error("spacer fail: " + err.Error())
		return
	}

	// ---- settings ----
	sgb, _ := walk.NewGroupBox(center)
	_ = sgb.SetTitle("设置")
	grid := walk.NewGridLayout()
	_ = sgb.SetLayout(grid)
	_ = grid.SetColumnStretchFactor(1, 1)

	row := 0
	addRow := func(label string, w walk.Widget) {
		lbl, _ := walk.NewLabel(sgb)
		_ = lbl.SetText(label)
		_ = grid.SetRange(lbl, walk.Rectangle{X: 0, Y: row, Width: 1, Height: 1})
		_ = grid.SetRange(w, walk.Rectangle{X: 1, Y: row, Width: 1, Height: 1})
		row++
	}
	span := func(w walk.Widget) {
		_ = grid.SetRange(w, walk.Rectangle{X: 0, Y: row, Width: 2, Height: 1})
		row++
	}

	// Numeric fields use LineEdit (not NumberEdit) so the text stays
	// left-aligned via SetTextAlignment.
	leInterval, _ := walk.NewLineEdit(sgb)
	_ = leInterval.SetText(strconv.Itoa(g.cfg.CheckIntervalMinutes))
	_ = leInterval.SetTextAlignment(walk.AlignNear)
	addRow("定时时长(分钟)", leInterval)

	leLatency, _ := walk.NewLineEdit(sgb)
	_ = leLatency.SetText(strconv.Itoa(g.cfg.LatencyThresholdMs))
	_ = leLatency.SetTextAlignment(walk.AlignNear)
	addRow("延时阈值(ms)", leLatency)

	leGroup, _ := walk.NewLineEdit(sgb)
	_ = leGroup.SetText(g.cfg.TargetGroup)
	addRow("目标策略组", leGroup)

	leURL, _ := walk.NewLineEdit(sgb)
	_ = leURL.SetText(g.cfg.ControllerURL)
	addRow("API 地址", leURL)

	leSecret, _ := walk.NewLineEdit(sgb)
	_ = leSecret.SetText(g.cfg.Secret)
	leSecret.SetPasswordMode(true)
	addRow("密钥", leSecret)

	cbCountry, _ := walk.NewCheckBox(sgb)
	_ = cbCountry.SetText("优先同国家")
	cbCountry.SetChecked(g.cfg.PreferSameCountry)
	span(cbCountry)

	cbQuiet, _ := walk.NewCheckBox(sgb)
	_ = cbQuiet.SetText("静默模式(不弹窗不关代理)")
	cbQuiet.SetChecked(g.cfg.TestMode)
	span(cbQuiet)

	cbAuto, _ := walk.NewCheckBox(sgb)
	_ = cbAuto.SetText("开机自启(随系统代理开关)")
	cbAuto.SetChecked(autostart.IsEnabled())
	span(cbAuto)

	bar, _ := walk.NewComposite(sgb)
	_ = bar.SetLayout(walk.NewHBoxLayout())
	_, _ = walk.NewHSpacer(bar)
	btnSave, _ := walk.NewPushButton(bar)
	_ = btnSave.SetText("保存设置")
	btnReload, _ := walk.NewPushButton(bar)
	_ = btnReload.SetText("重载配置")
	btnCheck, _ := walk.NewPushButton(bar)
	_ = btnCheck.SetText("立即检查")
	btnLog, _ := walk.NewPushButton(bar)
	_ = btnLog.SetText("日志目录")
	btnTest, _ := walk.NewPushButton(bar)
	_ = btnTest.SetText("测试连接")
	_, _ = walk.NewHSpacer(bar)
	span(bar)

	status, _ := walk.NewLabel(sgb)
	_ = status.SetText("")
	_ = status.SetTextAlignment(walk.AlignCenter)
	span(status)

	// ---- log ----
	lgb, _ := walk.NewGroupBox(center)
	_ = lgb.SetTitle("运行日志")
	_ = lgb.SetLayout(walk.NewVBoxLayout())
	logEdit, _ := walk.NewTextEdit(lgb)
	_ = logEdit.SetReadOnly(true)

	// ---- wiring ----
	readInto := func() bool {
		iv, err1 := strconv.Atoi(strings.TrimSpace(leInterval.Text()))
		lv, err2 := strconv.Atoi(strings.TrimSpace(leLatency.Text()))
		if err1 != nil || iv < 1 || iv > 120 {
			_ = status.SetText("定时时长无效(1-120)")
			return false
		}
		if err2 != nil || lv < 50 || lv > 2000 {
			_ = status.SetText("延时阈值无效(50-2000)")
			return false
		}
		g.cfg.CheckIntervalMinutes = iv
		g.cfg.LatencyThresholdMs = lv
		g.cfg.TargetGroup = leGroup.Text()
		g.cfg.ControllerURL = leURL.Text()
		g.cfg.Secret = leSecret.Text()
		g.cfg.PreferSameCountry = cbCountry.Checked()
		g.cfg.TestMode = cbQuiet.Checked()
		return true
	}
	writeFrom := func(c config.Config) {
		_ = leInterval.SetText(strconv.Itoa(c.CheckIntervalMinutes))
		_ = leLatency.SetText(strconv.Itoa(c.LatencyThresholdMs))
		_ = leGroup.SetText(c.TargetGroup)
		_ = leURL.SetText(c.ControllerURL)
		_ = leSecret.SetText(c.Secret)
		cbCountry.SetChecked(c.PreferSameCountry)
		cbQuiet.SetChecked(c.TestMode)
		cbAuto.SetChecked(autostart.IsEnabled())
	}

	btnSave.Clicked().Attach(func() {
		if !readInto() {
			return
		}
		if err := config.Save(g.dir, g.cfg); err != nil {
			_ = status.SetText("保存失败: " + err.Error())
			return
		}
		_ = status.SetText("已保存 ✓")
		g.ag.Kick()
	})
	btnReload.Clicked().Attach(func() {
		g.loading = true
		g.cfg = g.ag.Load()
		writeFrom(g.cfg)
		g.loading = false
		_ = status.SetText("已重载 ✓")
	})
	btnCheck.Clicked().Attach(func() {
		_ = status.SetText("已触发检查…")
		g.ag.Kick()
	})
	btnLog.Clicked().Attach(func() {
		if err := exec.Command("explorer.exe", filepath.Join(g.dir, "logs")).Start(); err != nil {
			_ = status.SetText("打开日志目录失败: " + err.Error())
		}
	})
	btnTest.Clicked().Attach(func() {
		to := g.cfg.HTTPTimeoutMs
		if to <= 0 {
			to = 5000
		}
		cli := api.New(strings.TrimSpace(leURL.Text()), leSecret.Text(), time.Duration(to)*time.Millisecond)
		ver, err := cli.VersionString()
		if err != nil {
			_ = status.SetText("连接失败: " + err.Error())
			return
		}
		pr, err := cli.GetProxies()
		if err != nil {
			_ = status.SetText("连接失败: " + err.Error())
			return
		}
		_ = status.SetText(fmt.Sprintf("连接正常：内核 %s，策略组 %d 个", ver, len(pr.Proxies)))
	})
	cbAuto.CheckedChanged().Attach(func() {
		if g.loading {
			return
		}
		if cbAuto.Checked() {
			if err := autostart.Enable(autostart.ExePath()); err != nil {
				_ = status.SetText("自启操作失败: " + err.Error())
				g.loading = true
				cbAuto.SetChecked(autostart.IsEnabled())
				g.loading = false
				return
			}
			_ = status.SetText("已启用开机自启 ✓")
		} else {
			_ = autostart.Disable()
			_ = status.SetText("已关闭开机自启 ✓")
		}
	})

	// ---- tray ----
	if ni, err := walk.NewNotifyIcon(mw); err == nil {
		_ = ni.SetToolTip("VerPinG")
		if icon != nil {
			_ = ni.SetIcon(icon)
		}
		// Left click on the tray icon opens the panel.
		ni.MouseDown().Attach(func(x, y int, button walk.MouseButton) {
			if button != walk.LeftButton {
				return
			}
			mw.Show()
			_ = mw.Activate()
		})
		menu := ni.ContextMenu()
		actOpen := walk.NewAction()
		_ = actOpen.SetText("打开界面")
		actOpen.Triggered().Attach(func() {
			mw.Show()
			_ = mw.Activate()
		})
		actHide := walk.NewAction()
		_ = actHide.SetText("隐藏界面")
		actHide.Triggered().Attach(func() { mw.Hide() })
		actQuit := walk.NewAction()
		_ = actQuit.SetText("退出")
		actQuit.Triggered().Attach(func() {
			g.quitting = true
			_ = ni.Dispose()
			walk.App().Exit(0)
		})
		_ = menu.Actions().Add(actOpen)
		_ = menu.Actions().Add(actHide)
		_ = menu.Actions().Add(walk.NewSeparatorAction())
		_ = menu.Actions().Add(actQuit)
		_ = ni.SetVisible(true)
	} else {
		logger.Error("create notify icon fail: " + err.Error())
	}

	mw.Closing().Attach(func(canceled *bool, reason walk.CloseReason) {
		if !g.quitting {
			*canceled = true
			mw.Hide()
		}
	})

	// ---- periodic log refresh (append-only so the wheel keeps working) ----
	// VERPING_NO_GUI_LOG=1 skips touching the TextEdit entirely; file logging
	// still runs. Diagnostic aid for isolating a system default beep.
	noGuiLog := os.Getenv("VERPING_NO_GUI_LOG") == "1"
	if noGuiLog {
		logger.Warn("[diag] GUI log panel refresh disabled (VERPING_NO_GUI_LOG=1)")
	}
	stopRefresh := make(chan struct{})
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		var last string
		for {
			select {
			case <-stopRefresh:
				return
			case <-ticker.C:
				txt := logger.ReadTail(20000)
				if txt == last {
					continue
				}
				last = txt
				if noGuiLog {
					continue
				}
				mw.Synchronize(func() {
					setLogText(logEdit, txt, editAtBottom(logEdit))
				})
			}
		}
	}()

	if show {
		mw.Show()
	}
	mw.Run()
	close(stopRefresh)
}

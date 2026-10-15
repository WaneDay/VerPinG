package agent

import (
	"sync"
	"time"

	"verping/internal/config"
	"verping/internal/logger"
	"verping/internal/switcher"
	"verping/internal/sysproxy"
)

const idlePoll = 5 * time.Second

// Agent runs the periodic switch loop, gated by the Windows system proxy.
type Agent struct {
	dir  string
	stop chan struct{}
	kick chan struct{}
	once sync.Once
}

// New creates an Agent rooted at dir.
func New(dir string) *Agent {
	return &Agent{
		dir:  dir,
		stop: make(chan struct{}),
		kick: make(chan struct{}, 1),
	}
}

// Load reads the current config from disk.
func (a *Agent) Load() config.Config {
	c, err := config.Load(a.dir)
	if err != nil {
		logger.Error("load config fail: " + err.Error())
		return config.Default()
	}
	return c
}

// Start launches the background loop.
func (a *Agent) Start() {
	go a.loop()
}

// Stop terminates the background loop.
func (a *Agent) Stop() {
	a.once.Do(func() { close(a.stop) })
}

// Kick requests an immediate check.
func (a *Agent) Kick() {
	select {
	case a.kick <- struct{}{}:
	default:
	}
}

func (a *Agent) loop() {
	var lastState *bool
	for {
		cfg := a.Load()
		on := sysproxy.Enabled()
		if lastState == nil || *lastState != on {
			if on {
				logger.Info("system proxy ON")
			} else {
				logger.Info("system proxy OFF, waiting")
			}
			lastState = &on
		}

		var wait time.Duration
		if !on {
			wait = idlePoll
		} else {
			switcher.RunOnce(cfg)
			wait = time.Duration(cfg.CheckIntervalMinutes) * time.Minute
		}

		// A kick that arrived while RunOnce was still running would otherwise
		// make sleep() return instantly and trigger a redundant back-to-back
		// check (and a second switch). Collapse those before waiting.
		a.drainKick()

		if a.sleep(wait) {
			return
		}
	}
}

// drainKick discards any pending kick without blocking.
func (a *Agent) drainKick() {
	for {
		select {
		case <-a.kick:
		default:
			return
		}
	}
}

func (a *Agent) sleep(d time.Duration) (stopped bool) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-a.stop:
		return true
	case <-a.kick:
		return false
	case <-t.C:
		return false
	}
}

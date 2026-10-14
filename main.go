package main

import (
	"os"
	"path/filepath"

	"verping/internal/agent"
	"verping/internal/config"
	"verping/internal/gui"
	"verping/internal/logger"
	"verping/internal/single"
)

func main() {
	h, ok := single.Acquire()
	if !ok {
		return
	}
	defer single.Release(h)

	exe, err := os.Executable()
	if err != nil {
		return
	}
	exeDir := filepath.Dir(exe)
	dir := config.DataDir(exeDir)

	_ = logger.Init(filepath.Join(dir, "logs"))
	logger.Info("app start")

	show := false
	for _, a := range os.Args[1:] {
		if a == "--show" {
			show = true
		}
	}

	ag := agent.New(dir)
	ag.Start()

	gui.Run(dir, exeDir, show, ag)

	ag.Stop()
	logger.Info("app exit")
}

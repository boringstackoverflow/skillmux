package app

import (
	"os"
	"path/filepath"
	"syscall"
)

type Lock struct {
	file *os.File
}

func (a *App) Lock() (*Lock, error) {
	lockPath := filepath.Join(a.SkillmuxHome, "state", "skillmux.lock")
	if err := ensureDir(filepath.Dir(lockPath)); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		_ = f.Close()
		return nil, err
	}
	return &Lock{file: f}, nil
}

func (l *Lock) Close() error {
	if l == nil || l.file == nil {
		return nil
	}
	err1 := syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN)
	err2 := l.file.Close()
	if err1 != nil {
		return err1
	}
	return err2
}

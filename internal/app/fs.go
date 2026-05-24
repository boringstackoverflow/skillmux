package app

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"
)

func ensureDir(path string) error {
	return os.MkdirAll(path, 0o755)
}

func removePath(path string) error {
	if _, err := os.Lstat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	return os.RemoveAll(path)
}

func replaceSymlink(linkPath, target string) error {
	if err := ensureDir(filepath.Dir(linkPath)); err != nil {
		return err
	}
	if err := removePath(linkPath); err != nil {
		return err
	}
	return os.Symlink(target, linkPath)
}

func copyPath(src, dst string) error {
	info, err := os.Lstat(src)
	if err != nil {
		return err
	}
	switch {
	case info.Mode()&os.ModeSymlink != 0:
		target, err := os.Readlink(src)
		if err != nil {
			return err
		}
		if err := ensureDir(filepath.Dir(dst)); err != nil {
			return err
		}
		return os.Symlink(target, dst)
	case info.IsDir():
		return copyDir(src, dst)
	default:
		return copyFile(src, dst, info.Mode())
	}
}

func copyDir(src, dst string) error {
	info, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if err := ensureDir(dst); err != nil {
		return err
	}
	_ = os.Chmod(dst, info.Mode().Perm())
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := copyPath(filepath.Join(src, entry.Name()), filepath.Join(dst, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

func copyFile(src, dst string, mode fs.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := ensureDir(filepath.Dir(dst)); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode.Perm())
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

func digestPath(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	_, _ = h.Write([]byte(info.Mode().String()))
	_, _ = h.Write([]byte{0})
	if info.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(path)
		if err != nil {
			return "", err
		}
		_, _ = h.Write([]byte(target))
		return hex.EncodeToString(h.Sum(nil)), nil
	}
	if !info.IsDir() {
		f, err := os.Open(path)
		if err != nil {
			return "", err
		}
		defer f.Close()
		if _, err := io.Copy(h, f); err != nil {
			return "", err
		}
		return hex.EncodeToString(h.Sum(nil)), nil
	}
	var rels []string
	err = filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p == path {
			return nil
		}
		rel, err := filepath.Rel(path, p)
		if err != nil {
			return err
		}
		rels = append(rels, rel)
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(rels)
	for _, rel := range rels {
		p := filepath.Join(path, rel)
		info, err := os.Lstat(p)
		if err != nil {
			return "", err
		}
		_, _ = h.Write([]byte(rel))
		_, _ = h.Write([]byte{0})
		_, _ = h.Write([]byte(info.Mode().String()))
		_, _ = h.Write([]byte{0})
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(p)
			if err != nil {
				return "", err
			}
			_, _ = h.Write([]byte(target))
		} else if !info.IsDir() {
			f, err := os.Open(p)
			if err != nil {
				return "", err
			}
			if _, err := io.Copy(h, f); err != nil {
				_ = f.Close()
				return "", err
			}
			_ = f.Close()
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func timestampID(reason string) string {
	return time.Now().UTC().Format("20060102T150405.000000000Z") + "-" + reason
}

func latestDir(path string) (string, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return "", err
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	if len(names) == 0 {
		return "", os.ErrNotExist
	}
	sort.Strings(names)
	return filepath.Join(path, names[len(names)-1]), nil
}

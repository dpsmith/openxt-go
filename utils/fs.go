package utils

import (
	"errors"
	"io"
	"os"
	"path/filepath"
)

var (
	ErrNotDir = errors.New("not a directory")
)

func Exists(path string) bool {
	_, err := os.Stat(path)

	return !os.IsNotExist(err)
}

func IsDir(path string) bool {
	stat, err := os.Stat(path)
	if err != nil {
		return false
	}

	return stat.IsDir()
}

// MoveDir moves a directory from src to dst across file systems.
func MoveDir(src, dst string) error {
	if !IsDir(src) {
		return ErrNotDir
	}

	if err := os.MkdirAll(dst, os.ModePerm); err != nil {
		return err
	}

	d, err := os.Open(src)
	if err != nil {
		return err
	}

	entries, err := d.Readdir(0)
	if err != nil {
		return err
	}
	d.Close()

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if IsDir(srcPath) {
			if err := MoveDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err := MoveFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}

	return os.Remove(src)
}

// MoveFile moves a file from src to dst across file systems.
func MoveFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	if _, err := io.Copy(destFile, sourceFile); err != nil {
		return err
	}

	return os.Remove(src)
}

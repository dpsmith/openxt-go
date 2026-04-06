// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright 2026 Apertus Solutions, LLC
//
// prov-agent is the OpenXT system provisioning agent.
//
// Build with Argo transport:
//
//	go build -tags argo ./cmd/prov-agent
//
// Build with a native D-Bus socket (default):
//
//	go build ./cmd/prov-agent
package main

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"

	"github.com/openxt/openxt-go/agent/provisioner"
	config "github.com/openxt/openxt-go/utils/ini"
	flag "github.com/spf13/pflag"
)

const version = "0.1.0"

type runtime struct {
	mu     sync.Mutex
	prov   *provisioner.Provisioner
	conf   string
}

func main() {
	configFile := flag.StringP("conf", "c", "/etc/ufo/provisioner.cfg", "path to provisioner config")
	showVersion := flag.BoolP("version", "V", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("prov-agent %s\n", version)
		os.Exit(0)
	}

	setDefaults()

	if err := config.ReadConfig(*configFile); err != nil {
		fmt.Fprintf(os.Stderr, "unable to read config file %s: %v\n", *configFile, err)
		os.Exit(1)
	}

	dbFile := config.Get("provisioner", "db-file")
	if dbFile  == "" {
		fmt.Fprintf(os.Stderr, "config file %s missing db-file entry\n", *configFile)
		os.Exit(1)
	}

	if err := ensureLayout(); err != nil {
		fmt.Fprintf(os.Stderr, "failed creating service directories: %v\n", err)
		os.Exit(1)
	}

	prov, err := setupProvisioner(dbFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}

	rt := &runtime{
		conf:  *configFile,
		prov:   prov,
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	exitCh := make(chan int, 1)
	go rt.handleSignals(sigCh, exitCh)

	code := <-exitCh
	if err := rt.shutdownAndPersist(); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		if code == 0 {
			code = 1
		}
	}
	os.Exit(code)
}

func setDefaults() {
	config.SetDefault("paths", "tftp-dir", "/srv/tftp")
	config.SetDefault("paths", "http-dir", "/srv/www")
	config.SetDefault("paths", "isos-dir", "/incoming/isos")
	config.SetDefault("provisioner", "db-file", "/srv/provisioner/db")
	config.SetDefault("provisioner", "dbus-addr", defaultBusAddr)
}

func ensureLayout() error {
	httpDir := config.Get("paths", "http-dir")
	tftpDir := config.Get("paths", "tftp-dir")
	dirs := []string{
		filepath.Join(httpDir, provisioner.AnswerDir),
		filepath.Join(httpDir, provisioner.DistrosDir),
		filepath.Join(tftpDir, provisioner.DistrosDir),
		filepath.Join(tftpDir, provisioner.SystemsDir),
		config.Get("paths", "isos-dir"),
		filepath.Dir(config.Get("provisioner", "db-file")),
	}
	for _, dir := range dirs {
		if dir == "" || dir == "." {
			continue
		}
		if err := os.MkdirAll(dir, 0750); err != nil {
			return err
		}
	}
	return nil
}

func setupProvisioner(dbFile string) (*provisioner.Provisioner, error) {
	var opts []provisioner.Option
	if config.Exists("paths", "template-dir") {
		path := config.Get("paths", "template-dir")
		opts = []provisioner.Option{
			provisioner.WithSystemTemplate(path + "/system.template"),
			provisioner.WithDistroTemplate(path + "/distro.template"),
		}
	}
	prov, err := provisioner.NewProvisioner(
		dbFile,
		config.Get("paths", "tftp-dir"),
		config.Get("paths", "http-dir"),
		config.Get("paths", "isos-dir"),
		opts...,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to allocate provisioner: %v", err)
	}

	addr := config.Get("provisioner", "dbus-addr")
	if addr == "" {
		return nil, fmt.Errorf("missing dbus address in config")
	}

	conn, err := connectBus(addr)
	if err != nil {
		return nil, fmt.Errorf("dbus connect %s: %v", addr, err)
	}

	if err := prov.DBusRegister(conn); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return prov, nil
}

func (rt *runtime) handleSignals(sigCh <-chan os.Signal, exitCh chan<- int) {
	for s := range sigCh {
		switch s {
		case syscall.SIGHUP:
			if err := rt.reload(); err != nil {
				fmt.Fprintf(os.Stderr, "reload failed: %v\n", err)
				exitCh <- 1
				return
			}
		case syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT:
			exitCh <- 0
			return
		default:
			fmt.Fprintf(os.Stderr, "unknown signal %v, exiting\n", s)
			exitCh <- 1
			return
		}
	}
}

func (rt *runtime) reload() error {
	if err := config.ReadConfig(rt.conf); err != nil {
		return fmt.Errorf("re-read config %s: %v", rt.conf, err)
	}
	dbFile := config.Get("provisioner", "db-file")
	if dbFile == "" {
		return fmt.Errorf("config file %s missing db-file entry", rt.conf)
	}
	if err := ensureLayout(); err != nil {
		return err
	}

	if err := rt.shutdown(); err != nil {
		return err
	}

	prov, err := setupProvisioner(dbFile)
	if err != nil {
		return fmt.Errorf("starting new instance: %v", err)
	}

	rt.mu.Lock()
	rt.prov = prov
	rt.mu.Unlock()
	return nil
}

func (rt *runtime) shutdown() error {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	if rt.prov == nil {
		return nil
	}

	err := rt.prov.Shutdown()
	rt.prov = nil
	if err != nil {
		return fmt.Errorf("failed to shutdown provisioner: %v", err)
	}
	return nil
}

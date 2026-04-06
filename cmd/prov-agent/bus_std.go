//go:build !argo
// +build !argo

// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright 2026 Apertus Solutions, LLC
package main

import (
	"fmt"
	"strings"

	godbus "github.com/godbus/dbus/v5"
)

// defaultBusAddr is the host system bus.
const defaultBusAddr = "unix:path=/var/run/dbus/system_bus_socket"

func connectBus(addr string) (*godbus.Conn, error) {
	if strings.HasPrefix(addr, "argo:") {
		return nil, fmt.Errorf("argo address %q requires building with -tags argo", addr)
	}
	return godbus.Connect(addr)
}

//go:build argo
// +build argo

// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright 2026 Apertus Solutions, LLC
package main

import (
	godbus "github.com/godbus/dbus/v5"
	argo "github.com/openxt/openxt-go/argo/dbus"
)

// defaultBusAddr is the OpenXT platform bus over Argo.
const defaultBusAddr = "argo:domain=0,port=5555"

func connectBus(addr string) (*godbus.Conn, error) {
	return argo.Connect(addr)
}

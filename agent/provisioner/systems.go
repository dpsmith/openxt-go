// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright 2026 Apertus Solutions, LLC
package provisioner

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/introspect"
)

const (
	SystemInterface  = "org.openxt.agent.provisioner.system"
	SystemObjectPath = dbus.ObjectPath("/org/openxt/agent/provisioner/system")
)

var (
	SystemsDir string = "systems"
)

// System is one PXE client: a MAC address plus the distro and answer
// file used to install it.
type System struct {
	Name    string
	MacAddr net.HardwareAddr
	Distro  string
	Answers string

	distro  *Distro
	answers *AnswerFile
}

func (s *System) objectPath() dbus.ObjectPath {
	return SystemObjectPath + dbus.ObjectPath("/"+s.Name)
}

func (s *System) MarshalJSON() ([]byte, error) {
	mac := ""
	if len(s.MacAddr) > 0 {
		mac = s.MacAddr.String()
	}
	t := struct {
		Name    string
		MacAddr string
		Distro  string
		Answers string
	}{
		Name:    s.Name,
		MacAddr: mac,
		Distro:  s.Distro,
		Answers: s.Answers,
	}
	return json.Marshal(&t)
}

func (s *System) UnmarshalJSON(b []byte) error {
	t := struct {
		Name    string
		MacAddr string
		Distro  string
		Answers string
	}{}
	if err := json.Unmarshal(b, &t); err != nil {
		return err
	}

	if t.Name != "" {
		name, err := validateName(t.Name)
		if err != nil {
			return fmt.Errorf("system name: %v", err)
		}
		s.Name = name
	} else {
		s.Name = ""
	}

	if strings.TrimSpace(t.MacAddr) == "" {
		s.MacAddr = nil
	} else {
		mac, err := parseSystemMAC(t.MacAddr)
		if err != nil {
			return fmt.Errorf("system %s MAC: %v", t.Name, err)
		}
		s.MacAddr = mac
	}

	s.Distro = t.Distro
	s.Answers = t.Answers
	s.distro = nil
	s.answers = nil
	return nil
}

func parseSystemMAC(addr string) (net.HardwareAddr, error) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return nil, fmt.Errorf("MAC address is empty")
	}
	mac, err := net.ParseMAC(addr)
	if err != nil {
		return nil, err
	}
	if len(mac) != 6 {
		return nil, fmt.Errorf("MAC address must be 6 bytes, got %d", len(mac))
	}
	for _, b := range mac {
		if b != 0 {
			return mac, nil
		}
	}
	return nil, fmt.Errorf("MAC address is zero")
}

func (s *System) tftpFileName() (string, error) {
	if len(s.MacAddr) != 6 {
		return "", fmt.Errorf("system %q has no MAC address", s.Name)
	}
	return strings.ToLower(strings.ReplaceAll(s.MacAddr.String(), ":", "-")), nil
}

func (s *System) WriteTftpConfig(root string, t *template.Template) error {
	if s.distro == nil || s.answers == nil {
		return fmt.Errorf("system %q is not fully configured", s.Name)
	}
	if t == nil {
		return fmt.Errorf("nil template provided")
	}

	aFile, err := s.answers.FileName()
	if err != nil {
		return err
	}
	macName, err := s.tftpFileName()
	if err != nil {
		return err
	}

	contents := struct {
		AnswerFile string
		Distro     string
	}{
		AnswerFile: string(aFile),
		Distro:     s.Distro,
	}
	if strings.Contains(contents.AnswerFile, "{{") || strings.Contains(contents.Distro, "{{") {
		return fmt.Errorf("system %q field contains template metacharacters", s.Name)
	}

	filePath, err := rootedJoin(root, SystemsDir, macName)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(filePath), 0750); err != nil {
		return err
	}

	file, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0640)
	if err != nil {
		return err
	}
	defer file.Close()
	return t.Execute(file, contents)
}

const systemIntro = `
<node>
	<interface name="` + SystemInterface + `">
		<method name="Read">
			<arg direction="out" type="s"/>
		</method>
		<method name="SetMacAddr">
			<arg direction="in" type="s"/>
		</method>
		<method name="SetDistro">
			<arg direction="in" type="s"/>
		</method>
		<method name="SetAnswerFile">
			<arg direction="in" type="s"/>
		</method>
	</interface>` + introspect.IntrospectDataString + `</node> `

func (s *System) DBusRegister(conn *dbus.Conn) error {
	if _, err := validateName(s.Name); err != nil {
		return fmt.Errorf("system name: %v", err)
	}
	o := s.objectPath()
	if err := conn.Export(s, o, SystemInterface); err != nil {
		return fmt.Errorf("dbus export failed: %v", err)
	}
	if err := conn.Export(introspect.Introspectable(systemIntro),
		o, "org.freedesktop.DBus.Introspectable"); err != nil {
		return fmt.Errorf("dbus introspection export failed: %v", err)
	}
	return nil
}

func (s *System) DBusUnregister(conn *dbus.Conn) error {
	o := s.objectPath()
	_ = conn.Export(nil, o, "org.freedesktop.DBus.Introspectable")
	return conn.Export(nil, o, SystemInterface)
}

func (s *System) Read() (string, *dbus.Error) {
	jsonBytes, err := json.MarshalIndent(s, " ", "  ")
	if err != nil {
		return "", dbus.MakeFailedError(err)
	}
	return string(jsonBytes), nil
}

func (s *System) SetMacAddr(addr string) *dbus.Error {
	mac, err := parseSystemMAC(addr)
	if err != nil {
		return dbus.MakeFailedError(err)
	}
	s.MacAddr = mac
	return nil
}

func (s *System) SetDistro(d string) *dbus.Error {
	s.Distro = strings.TrimSpace(d)
	s.distro = nil
	return nil
}

func (s *System) SetAnswerFile(a string) *dbus.Error {
	a = strings.TrimSpace(a)
	if a == "" {
		s.Answers = ""
		s.answers = nil
		return nil
	}
	name, err := validateName(a)
	if err != nil {
		return dbus.MakeFailedError(err)
	}
	s.Answers = name
	s.answers = nil
	return nil
}

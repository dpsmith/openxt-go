// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright 2026 Apertus Solutions, LLC
package provisioner

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/introspect"
	"github.com/godbus/dbus/v5/prop"
	"github.com/openxt/openxt-go/installer"
)

const (
	AnswerFileInterface  = "org.openxt.agent.provisioner.answerfile"
	AnswerFileObjectPath = dbus.ObjectPath("/org/openxt/agent/provisioner/answerfile")
)

var (
	AnswerDir    = "answers"
	AnswerSuffix = ".ans"
)

// Vhd is one virtual disk image referenced by an answer file.
type Vhd struct {
	Label    string
	Compress string
	Sources  []string
}

// VmVhd maps a VM disk index onto a Vhd label.
type VmVhd struct {
	Index string
	Label string
}

// Vm is one guest to install from an answer file.
type Vm struct {
	Source string
	Vhds   []VmVhd
}

// AnswerFile is the provisioner's view of an OpenXT installer answer file.
type AnswerFile struct {
	Name   string
	Source struct {
		Type   string
		Url    string
		Verify bool
		Oem    bool
	}
	Interactive   bool
	Mode          string
	PrimaryDisk   string
	PartitionMode string
	InstallGpt    string
	EulaAccept    string
	Language      string
	Keyboard      string
	LicenseKey    string
	Netif         struct {
		Mode    string
		Addr    string
		Mask    string
		Gateway string
		Dns     string
	}
	Password struct {
		System   string
		Recovery string
		TPM      string
	}
	EnableSsh        bool
	SkipReady        bool
	AllowDevRepoCert bool
	Vhds             []Vhd
	Vms              []Vm
	PreInstall       []string
	PostInstall      []string

	mu sync.RWMutex
}

func (a *AnswerFile) objectPath() dbus.ObjectPath {
	return AnswerFileObjectPath + dbus.ObjectPath("/"+a.Name)
}

func (a *AnswerFile) FileName() ([]byte, error) {
	name, err := validateName(a.Name)
	if err != nil {
		return nil, err
	}
	return []byte(name + AnswerSuffix), nil
}

func (a *AnswerFile) Marshal(funcs *ShellFuncList) ([]byte, error) {
	f := installer.NewAnswerFile(a.Source.Type, a.Source.Url, a.Source.Verify, a.Source.Oem)

	if err := f.SetTag(installer.InteractiveTag, a.Interactive); err != nil {
		return nil, err
	}

	eula := a.EulaAccept
	if eula == "" {
		eula = "yes"
	}
	if err := f.SetEula(eula); err != nil {
		return nil, err
	}

	if a.Mode != "" {
		if err := f.SetTag(installer.ModeTag, a.Mode); err != nil {
			return nil, err
		}
	}
	if a.PrimaryDisk != "" {
		if err := f.SetTag(installer.PrimaryDiskTag, a.PrimaryDisk); err != nil {
			return nil, err
		}
	}
	if a.PartitionMode != "" {
		if err := f.SetTag(installer.PartitionModeTag, a.PartitionMode); err != nil {
			return nil, err
		}
	}
	if a.InstallGpt != "" {
		if err := f.SetTag(installer.InstallGptTag, a.InstallGpt); err != nil {
			return nil, err
		}
	}
	if err := f.SetTag(installer.EnableSshTag, a.EnableSsh); err != nil {
		return nil, err
	}
	if a.SkipReady {
		if err := f.SetTag(installer.SkipReadyTag, true); err != nil {
			return nil, err
		}
	}
	if a.Language != "" {
		if err := f.SetLanguage(a.Language); err != nil {
			return nil, err
		}
	}
	if a.Keyboard != "" {
		if err := f.SetKeyboard(a.Keyboard); err != nil {
			return nil, err
		}
	}
	if a.LicenseKey != "" {
		if err := f.SetLicenseKey(a.LicenseKey); err != nil {
			return nil, err
		}
	}
	if a.AllowDevRepoCert {
		if err := f.SetAllowDevRepoCert(true); err != nil {
			return nil, err
		}
	}

	if err := marshalPasswords(f, a); err != nil {
		return nil, err
	}

	if a.Netif.Mode != "" {
		attrs := []*installer.Attr{installer.NewAttr("mode", a.Netif.Mode)}
		if a.Netif.Addr != "" {
			attrs = append(attrs, installer.NewAttr("address", a.Netif.Addr))
		}
		if a.Netif.Mask != "" {
			attrs = append(attrs, installer.NewAttr("netmask", a.Netif.Mask))
		}
		if a.Netif.Gateway != "" {
			attrs = append(attrs, installer.NewAttr("gateway", a.Netif.Gateway))
		}
		if a.Netif.Dns != "" {
			attrs = append(attrs, installer.NewAttr("dns", a.Netif.Dns))
		}
		if err := f.SetNetwork(attrs); err != nil {
			return nil, err
		}
	}

	for _, vhd := range a.Vhds {
		if err := f.AddVhd(vhd.Label, vhd.Compress, vhd.Sources); err != nil {
			return nil, err
		}
	}
	for _, vm := range a.Vms {
		labels := make([]string, 0, len(vm.Vhds))
		for _, disk := range vm.Vhds {
			labels = append(labels, disk.Label)
		}
		if err := f.AddVm(vm.Source, labels); err != nil {
			return nil, err
		}
	}

	if len(a.PreInstall) > 0 {
		if funcs == nil {
			return nil, fmt.Errorf("preinstall functions requested but function list is nil")
		}
		b, err := funcs.MarshalList(a.PreInstall)
		if err != nil {
			return nil, err
		}
		if err := f.AddPreInstall(b); err != nil {
			return nil, err
		}
	}
	if len(a.PostInstall) > 0 {
		if funcs == nil {
			return nil, fmt.Errorf("postinstall functions requested but function list is nil")
		}
		b, err := funcs.MarshalList(a.PostInstall)
		if err != nil {
			return nil, err
		}
		if err := f.AddPostInstall(b); err != nil {
			return nil, err
		}
	}

	return f.Marshal()
}

func marshalPasswords(f *installer.AnswerFile, a *AnswerFile) error {
	switch strings.ToLower(strings.TrimSpace(a.Password.System)) {
	case "":
	case "none":
		if err := f.SetPasswordNone(); err != nil {
			return err
		}
	case "defer":
		if err := f.SetPasswordDeferred(); err != nil {
			return err
		}
	default:
		if err := f.SetPassword(a.Password.System); err != nil {
			return err
		}
	}
	if a.Password.Recovery != "" {
		if err := f.SetRecoveryPassword(a.Password.Recovery); err != nil {
			return err
		}
	}
	if a.Password.TPM != "" {
		if err := f.SetTPMOwnerPassword(a.Password.TPM); err != nil {
			return err
		}
	}
	return nil
}

func (a *AnswerFile) WriteFile(root string, funcs *ShellFuncList) error {
	name, err := validateName(a.Name)
	if err != nil {
		return err
	}

	filePath, err := rootedJoin(root, AnswerDir, name+AnswerSuffix)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(filePath), 0750); err != nil {
		return err
	}

	contents, err := a.Marshal(funcs)
	if err != nil {
		return err
	}

	return ioutilWriteFile(filePath, contents, 0640)
}

func ioutilWriteFile(path string, data []byte, perm os.FileMode) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(data)
	return err
}

func (a *AnswerFile) DbusPropUpdate(change *prop.Change) *dbus.Error {
	a.mu.Lock()
	defer a.mu.Unlock()

	pair := strings.Split(change.Name, "_")
	switch len(pair) {
	case 1:
		switch pair[0] {
		case "Interactive":
			a.Interactive = change.Value.(bool)
		case "Mode":
			a.Mode = change.Value.(string)
		case "PrimaryDisk":
			a.PrimaryDisk = change.Value.(string)
		case "PartitionMode":
			a.PartitionMode = change.Value.(string)
		case "InstallGpt":
			a.InstallGpt = change.Value.(string)
		case "EulaAccept":
			a.EulaAccept = change.Value.(string)
		case "Language":
			a.Language = change.Value.(string)
		case "Keyboard":
			a.Keyboard = change.Value.(string)
		case "LicenseKey":
			a.LicenseKey = change.Value.(string)
		case "EnableSsh":
			a.EnableSsh = change.Value.(bool)
		case "SkipReady":
			a.SkipReady = change.Value.(bool)
		case "AllowDevRepoCert":
			a.AllowDevRepoCert = change.Value.(bool)
		default:
			return dbus.MakeFailedError(fmt.Errorf("unknown property %s", change.Name))
		}
	case 2:
		switch pair[0] {
		case "Source":
			switch pair[1] {
			case "Type":
				a.Source.Type = change.Value.(string)
			case "Url":
				a.Source.Url = change.Value.(string)
			case "Verify":
				a.Source.Verify = change.Value.(bool)
			case "Oem":
				a.Source.Oem = change.Value.(bool)
			default:
				return dbus.MakeFailedError(fmt.Errorf("unknown property %s", change.Name))
			}
		case "Netif":
			switch pair[1] {
			case "Mode":
				a.Netif.Mode = change.Value.(string)
			case "Addr":
				a.Netif.Addr = change.Value.(string)
			case "Mask":
				a.Netif.Mask = change.Value.(string)
			case "Gateway":
				a.Netif.Gateway = change.Value.(string)
			case "Dns":
				a.Netif.Dns = change.Value.(string)
			default:
				return dbus.MakeFailedError(fmt.Errorf("unknown property %s", change.Name))
			}
		case "Password":
			switch pair[1] {
			case "System":
				a.Password.System = change.Value.(string)
			case "Recovery":
				a.Password.Recovery = change.Value.(string)
			case "TPM":
				a.Password.TPM = change.Value.(string)
			default:
				return dbus.MakeFailedError(fmt.Errorf("unknown property %s", change.Name))
			}
		default:
			return dbus.MakeFailedError(fmt.Errorf("unknown property prefix %s", pair[0]))
		}
	default:
		return dbus.MakeFailedError(fmt.Errorf("unknown property %s", change.Name))
	}

	return nil
}

func (a *AnswerFile) DBusRegister(conn *dbus.Conn) error {
	if _, err := validateName(a.Name); err != nil {
		return err
	}
	path := a.objectPath()

	fieldsMap := map[string][]string{
		"": []string{
			"Interactive",
			"Mode",
			"PrimaryDisk",
			"PartitionMode",
			"InstallGpt",
			"EulaAccept",
			"Language",
			"Keyboard",
			"LicenseKey",
			"EnableSsh",
			"SkipReady",
			"AllowDevRepoCert",
		},
		"Source":   []string{"Type", "Url", "Verify", "Oem"},
		"Netif":    []string{"Mode", "Addr", "Mask", "Gateway", "Dns"},
		"Password": []string{"System", "Recovery", "TPM"},
	}

	propsMap, err := BuildDBusPropMap(fieldsMap, a, "DbusPropUpdate")
	if err != nil {
		return err
	}
	for name, p := range propsMap {
		if strings.HasPrefix(name, "Password_") {
			p.Emit = prop.EmitFalse
		}
	}

	propsSpec := map[string]map[string]*prop.Prop{
		AnswerFileInterface: propsMap,
	}
	props := prop.New(conn, path, propsSpec)

	if err := conn.Export(a, path, AnswerFileInterface); err != nil {
		return fmt.Errorf("dbus export failed: %v", err)
	}

	node := &introspect.Node{
		Name: string(path),
		Interfaces: []introspect.Interface{
			introspect.IntrospectData,
			prop.IntrospectData,
			{
				Name:       AnswerFileInterface,
				Properties: props.Introspection(AnswerFileInterface),
				Methods: []introspect.Method{
					{Name: "ListVhds", Args: []introspect.Arg{{Name: "labels", Type: "as", Direction: "out"}}},
					{Name: "AddVhd", Args: []introspect.Arg{
						{Name: "label", Type: "s", Direction: "in"},
						{Name: "compress", Type: "s", Direction: "in"},
						{Name: "sources", Type: "as", Direction: "in"},
					}},
					{Name: "DelVhd", Args: []introspect.Arg{{Name: "label", Type: "s", Direction: "in"}}},
					{Name: "ListVms", Args: []introspect.Arg{{Name: "sources", Type: "as", Direction: "out"}}},
					{Name: "AddVm", Args: []introspect.Arg{
						{Name: "source", Type: "s", Direction: "in"},
						{Name: "labels", Type: "as", Direction: "in"},
					}},
					{Name: "DelVm", Args: []introspect.Arg{{Name: "source", Type: "s", Direction: "in"}}},
				},
			},
		},
	}

	return conn.Export(
		introspect.NewIntrospectable(node),
		path,
		"org.freedesktop.DBus.Introspectable",
	)
}

func (a *AnswerFile) DBusUnregister(conn *dbus.Conn) error {
	path := a.objectPath()
	_ = conn.Export(nil, path, AnswerFileInterface)
	return conn.Export(nil, path, "org.freedesktop.DBus.Introspectable")
}

func (a *AnswerFile) ListVhds() ([]string, *dbus.Error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	list := make([]string, 0, len(a.Vhds))
	for _, vhd := range a.Vhds {
		list = append(list, vhd.Label)
	}
	return list, nil
}

func (a *AnswerFile) AddVhd(label, compress string, sources []string) *dbus.Error {
	a.mu.Lock()
	defer a.mu.Unlock()

	name, err := validateName(label)
	if err != nil {
		return dbus.MakeFailedError(err)
	}
	for _, v := range a.Vhds {
		if v.Label == name {
			return dbus.MakeFailedError(fmt.Errorf("vhd already exists: %s", name))
		}
	}
	a.Vhds = append(a.Vhds, Vhd{Label: name, Compress: compress, Sources: append([]string(nil), sources...)})
	return nil
}

func (a *AnswerFile) DelVhd(label string) *dbus.Error {
	a.mu.Lock()
	defer a.mu.Unlock()

	for i, v := range a.Vhds {
		if v.Label == label {
			a.Vhds = append(a.Vhds[:i], a.Vhds[i+1:]...)
			return nil
		}
	}
	return dbus.MakeFailedError(fmt.Errorf("no such vhd %s", label))
}

func (a *AnswerFile) ListVms() ([]string, *dbus.Error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	list := make([]string, 0, len(a.Vms))
	for _, vm := range a.Vms {
		list = append(list, vm.Source)
	}
	return list, nil
}

func (a *AnswerFile) AddVm(source string, labels []string) *dbus.Error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if strings.TrimSpace(source) == "" {
		return dbus.MakeFailedError(fmt.Errorf("vm source is empty"))
	}
	disks := make([]VmVhd, 0, len(labels))
	for i, label := range labels {
		disks = append(disks, VmVhd{Index: fmt.Sprintf("%d", i), Label: label})
	}
	a.Vms = append(a.Vms, Vm{Source: source, Vhds: disks})
	return nil
}

func (a *AnswerFile) DelVm(source string) *dbus.Error {
	a.mu.Lock()
	defer a.mu.Unlock()

	for i, vm := range a.Vms {
		if vm.Source == source {
			a.Vms = append(a.Vms[:i], a.Vms[i+1:]...)
			return nil
		}
	}
	return dbus.MakeFailedError(fmt.Errorf("no such vm %s", source))
}

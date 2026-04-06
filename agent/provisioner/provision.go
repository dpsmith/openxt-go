// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright 2026 Apertus Solutions, LLC
package provisioner

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"text/template"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/introspect"
	"github.com/openxt/openxt-go/logging"
	"github.com/openxt/openxt-go/utils"
)

const (
	ProvisionerInterface  = "org.openxt.agent.provisioner"
	ProvisionerObjectPath = dbus.ObjectPath("/org/openxt/agent/provisioner")
)

type Config struct {
	Name           string
	TftpPath       string
	HttpPath       string
	IsosPath       string
	SystemTemplate string
	DistroTemplate string
	Functions      ShellFuncList
	Answers        map[string]*AnswerFile
	Distros        map[string]*Distro
	Systems        map[string]*System

	mu             sync.Mutex
	filePath       string
}

func NewConfig(path string) (*Config, error) {
	if _, err := os.Stat(path); err != nil {
		c := Config {
			Name:           "ufo-prov",
			Functions:      make(ShellFuncList),
			Answers:        make(map[string]*AnswerFile),
			Distros:        make(map[string]*Distro),
			Systems:        make(map[string]*System),
			DistroTemplate: "/usr/share/grub-pxe/distro.template",
			SystemTemplate: "/usr/share/grub-pxe/system.template",
			filePath:       path,
		}
		// Ensure db can be written before returning success
		if err := c.Flush(); err != nil {
			return nil, err
		}

		return &c, nil
	}
	
	config, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	c := Config{filePath: path}
	if err = json.Unmarshal(config, &c); err != nil {
		return nil, err
	}

	return &c, nil
}

func (c *Config) Flush() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	dir := filepath.Dir(c.filePath)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, " ", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.OpenFile(c.filePath+".tmp", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0640)
	if err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), c.filePath); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return nil
}

type Option func(*Config)

func WithSystemTemplate(path string) Option {
	return func(c *Config) {
		c.SystemTemplate = path
	}
}

func WithDistroTemplate(path string) Option {
	return func(c *Config) {
		c.DistroTemplate = path
	}
}

func WithName(name string) Option {
	return func(c *Config) {
		c.Name = name
	}
}

type Provisioner struct {
	mu             sync.Mutex
	config         *Config
	conn           *dbus.Conn
	log            *logging.SystemLogger
	systemTemplate *template.Template
	distroTemplate *template.Template
}

func coreSetup(p *Provisioner) error {
	var err error

	p.distroTemplate, err = template.ParseFiles(p.config.DistroTemplate)
	if err != nil {
		return fmt.Errorf("distro template parse failed: %v", err)
	}

	p.systemTemplate, err = template.ParseFiles(p.config.SystemTemplate)
	if err != nil {
		return fmt.Errorf("system template parse failed: %v", err)
	}

	for _, s := range p.config.Systems {
		if a, okay := p.config.Answers[s.Answers]; okay {
			s.answers = a
		} else {
			s.Answers = ""
			s.answers = nil
		}

		if d, okay := p.config.Distros[s.Distro]; okay {
			s.distro = d
		} else {
			s.Distro = ""
			s.distro = nil
		}
	}

	p.log = logging.NewSystemLogger(p.config.Name)

	return nil
}

func NewProvisioner(
	config, tftp, http, isos string,
	options ...Option,
) (*Provisioner, error) {
	c, err := NewConfig(config)
	if err != nil {
		return nil, err
	}

	c.TftpPath = tftp
	c.HttpPath = http
	c.IsosPath = isos

	for _, opt := range options {
		opt(&c)
	}

	if err := c.Flush(); err != nil {
		return nil, err
	}

	p := Provisioner{config: c}
	if err := coreSetup(&p); err != nil {
		return nil, err
	}

	return &p, nil
}

func (p *Provisioner) Shutdown() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.conn != nil {
		_ = p.unpublishDBus()
		_ = p.conn.Export(nil, ProvisionerObjectPath, ProvisionerInterface)
		_ = p.conn.Export(nil, ProvisionerObjectPath, "org.freedesktop.DBus.Introspectable")
		_, _ = p.conn.ReleaseName(ProvisionerInterface)
		_ = p.conn.Close()
		p.conn = nil
	}

	if p.log != nil {
		p.log.Close()
	}

	return p.config.Flush()
}

func emptyDir(path string) error {
	if err := os.MkdirAll(path, 0750); err != nil {
		return err
	}
	entries, err := ioutil.ReadDir(path)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if err := os.RemoveAll(filepath.Join(path, e.Name())); err != nil {
			return err
		}
	}
	return nil
}

func (p *Provisioner) syncAnswers() error {
	path, err := rootedJoin(p.config.HttpPath, AnswerDir)
	if err != nil {
		return err
	}
	if err := emptyDir(path); err != nil {
		err = fmt.Errorf("failed clearing answers dir (%s): %v", path, err)
		p.log.Err("%s", err.Error())
		return err
	}
	for n, d := range p.config.Answers {
		if err := d.WriteFile(p.config.HttpPath, &p.config.Functions); err != nil {
			err = fmt.Errorf("failed writing answer file %s: %v", n, err)
			p.log.Err("%s", err.Error())
			return err
		}
	}
	return nil
}

func (p *Provisioner) syncDistros() error {
	path := filepath.Join(p.config.IsosPath, "*")
	isos, err := filepath.Glob(path)
	if err != nil {
		err = fmt.Errorf("failed listing isos path (%s): %v", path, err)
		p.log.Err("%s", err.Error())
		return err
	}
	for _, i := range isos {
		name := filepath.Base(i)
		if ParseDistroName(name) == nil {
			continue
		}
		dst := filepath.Join(p.config.HttpPath, DistrosDir, name)
		if err := utils.MoveDir(i, dst); err != nil {
			err = fmt.Errorf("failed moving iso (%s) to distro directory: %v", name, err)
			p.log.Err("%s", err.Error())
			return err
		}
	}

	path = filepath.Join(p.config.HttpPath, DistrosDir, "*")
	distros, err := filepath.Glob(path)
	if err != nil {
		err = fmt.Errorf("failed listing distros path (%s): %v", path, err)
		p.log.Err("%s", err.Error())
		return err
	}
	for k := range p.config.Distros {
		delete(p.config.Distros, k)
	}
	for _, d := range distros {
		name := filepath.Base(d)
		if distro := NewDistro(name); distro != nil {
			p.config.Distros[name] = distro
		}
	}

	tftpDistros, err := rootedJoin(p.config.TftpPath, DistrosDir)
	if err != nil {
		return err
	}
	if err := emptyDir(tftpDistros); err != nil {
		err = fmt.Errorf("failed clearing distro dir (%s): %v", tftpDistros, err)
		p.log.Err("%s", err.Error())
		return err
	}
	for n, d := range p.config.Distros {
		if err := d.WriteTftpConfig(p.config.TftpPath, p.distroTemplate); err != nil {
			err = fmt.Errorf("failed writing tftp file for distro %s: %v", n, err)
			p.log.Err("%s", err.Error())
			return err
		}
	}
	return nil
}

func (p *Provisioner) syncSystems() error {
	seenMAC := make(map[string]string)
	for _, s := range p.config.Systems {
		if d, okay := p.config.Distros[s.Distro]; okay {
			s.distro = d
		} else {
			s.Distro = ""
			s.distro = nil
		}
		if a, okay := p.config.Answers[s.Answers]; okay {
			s.answers = a
		} else {
			s.Answers = ""
			s.answers = nil
		}
		if s.MacAddr != nil {
			key := s.MacAddr.String()
			if prev, exists := seenMAC[key]; exists {
				err := fmt.Errorf("duplicate MAC %s on systems %s and %s", key, prev, s.Name)
				p.log.Err("%s", err.Error())
				return err
			}
			seenMAC[key] = s.Name
		}
	}

	tftpSystems, err := rootedJoin(p.config.TftpPath, SystemsDir)
	if err != nil {
		return err
	}
	if err := emptyDir(tftpSystems); err != nil {
		err = fmt.Errorf("failed clearing system dir (%s): %v", tftpSystems, err)
		p.log.Err("%s", err.Error())
		return err
	}

	var failed []string
	for _, s := range p.config.Systems {
		if err := s.WriteTftpConfig(p.config.TftpPath, p.systemTemplate); err != nil {
			failed = append(failed, s.Name)
		}
	}
	if len(failed) > 0 {
		err := fmt.Errorf("failed writing tftp file for systems: %s", strings.Join(failed, ", "))
		p.log.Err("%s", err.Error())
		return err
	}
	return nil
}

func (p *Provisioner) syncAll() error {
	if err := p.syncAnswers(); err != nil {
		return err
	}
	if err := p.syncDistros(); err != nil {
		return err
	}
	return p.syncSystems()
}

const provIntro = `
<node>
	<interface name="` + ProvisionerInterface + `">
		<method name="ListAnswers">
			<arg direction="out" type="ao"/>
		</method>
		<method name="ListDistros">
			<arg direction="out" type="as"/>
		</method>
		<method name="ListSystems">
			<arg direction="out" type="ao"/>
		</method>
		<method name="AddAnswerFile">
			<arg direction="in" type="s"/>
			<arg direction="out" type="o"/>
		</method>
		<method name="DelAnswerFile">
			<arg direction="in" type="s"/>
		</method>
		<method name="AddSystem">
			<arg direction="in" type="s"/>
			<arg direction="out" type="o"/>
		</method>
		<method name="DelSystem">
			<arg direction="in" type="s"/>
		</method>
		<method name="ListFunctions">
			<arg direction="out" type="as"/>
		</method>
		<method name="AddFunction">
			<arg direction="in" type="s"/>
			<arg direction="in" type="s"/>
		</method>
		<method name="DelFunction">
			<arg direction="in" type="s"/>
		</method>
		<method name="SyncAnswers">
		</method>
		<method name="SyncDistros">
		</method>
		<method name="SyncSystems">
		</method>
		<method name="Sync">
		</method>
	</interface>` + introspect.IntrospectDataString + `</node> `

func (p *Provisioner) DBusRegister(conn *dbus.Conn) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if err := p.syncAll(); err != nil {
		err = fmt.Errorf("failed synchronizing files: %v", err)
		p.log.Err("%s", err.Error())
		return err
	}

	reply, err := conn.RequestName(ProvisionerInterface, dbus.NameFlagDoNotQueue)
	if err != nil {
		err = fmt.Errorf("dbus registration failed: %v", err)
		p.log.Err("%s", err.Error())
		return err
	}
	if reply != dbus.RequestNameReplyPrimaryOwner {
		err = fmt.Errorf("dbus service name already taken")
		p.log.Err("dbus register: %s", err.Error())
		return err
	}

	if err := conn.Export(p, ProvisionerObjectPath, ProvisionerInterface); err != nil {
		err = fmt.Errorf("dbus export failed: %v", err)
		p.log.Err("%s", err.Error())
		return err
	}
	if err := conn.Export(introspect.Introspectable(provIntro),
		ProvisionerObjectPath, "org.freedesktop.DBus.Introspectable"); err != nil {
		err = fmt.Errorf("dbus introspection export failed: %v", err)
		p.log.Err("%s", err.Error())
		return err
	}

	p.conn = conn
	return p.publishDBus()
}

func (p *Provisioner) publishDBus() error {
	for _, a := range p.config.Answers {
		if err := a.DBusRegister(p.conn); err != nil {
			err = fmt.Errorf("failed dbus registration for %s: %v", a.Name, err)
			p.log.Err("%s", err.Error())
			return err
		}
	}
	for _, s := range p.config.Systems {
		if err := s.DBusRegister(p.conn); err != nil {
			err = fmt.Errorf("failed dbus registration for %s: %v", s.Name, err)
			p.log.Err("%s", err.Error())
			return err
		}
	}
	return nil
}

func (p *Provisioner) unpublishDBus() error {
	var first error
	for _, a := range p.config.Answers {
		if err := a.DBusUnregister(p.conn); err != nil && first == nil {
			first = err
		}
	}
	for _, s := range p.config.Systems {
		if err := s.DBusUnregister(p.conn); err != nil && first == nil {
			first = err
		}
	}
	return first
}

func (p *Provisioner) ListAnswers() ([]dbus.ObjectPath, *dbus.Error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	list := make([]dbus.ObjectPath, 0, len(p.config.Answers))
	for _, a := range p.config.Answers {
		list = append(list, a.objectPath())
	}
	return list, nil
}

func (p *Provisioner) ListDistros() ([]string, *dbus.Error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	list := make([]string, 0, len(p.config.Distros))
	for _, d := range p.config.Distros {
		list = append(list, d.Name)
	}
	return list, nil
}

func (p *Provisioner) ListSystems() ([]dbus.ObjectPath, *dbus.Error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	list := make([]dbus.ObjectPath, 0, len(p.config.Systems))
	for _, s := range p.config.Systems {
		list = append(list, SystemObjectPath+dbus.ObjectPath("/"+s.Name))
	}
	return list, nil
}

func (p *Provisioner) AddAnswerFile(name string) (dbus.ObjectPath, *dbus.Error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	name, err := validateName(name)
	if err != nil {
		p.log.Err("AddAnswerFile: %s", err.Error())
		return "", dbus.MakeFailedError(err)
	}
	if _, exists := p.config.Answers[name]; exists {
		err := fmt.Errorf("answer file already exists: %s", name)
		p.log.Err("AddAnswerFile: %s", err.Error())
		return "", dbus.MakeFailedError(err)
	}

	a := AnswerFile{Name: name, EulaAccept: "yes"}
	if err := a.DBusRegister(p.conn); err != nil {
		err = fmt.Errorf("failed dbus registration for %s: %v", a.Name, err)
		p.log.Err("AddAnswerFile: %s", err.Error())
		return "", dbus.MakeFailedError(err)
	}
	p.config.Answers[name] = &a
	return a.objectPath(), nil
}

func (p *Provisioner) DelAnswerFile(name string) *dbus.Error {
	p.mu.Lock()
	defer p.mu.Unlock()

	name, err := validateName(name)
	if err != nil {
		p.log.Err("DelAnswerFile: %s", err.Error())
		return dbus.MakeFailedError(err)
	}
	a, exists := p.config.Answers[name]
	if !exists {
		err := fmt.Errorf("no such answerfile %s", name)
		p.log.Err("DelAnswerFile: %s", err.Error())
		return dbus.MakeFailedError(err)
	}
	if err := a.DBusUnregister(p.conn); err != nil {
		p.log.Err("DelAnswerFile: %s", err.Error())
		return dbus.MakeFailedError(err)
	}
	delete(p.config.Answers, name)
	return nil
}

func (p *Provisioner) AddSystem(name string) (dbus.ObjectPath, *dbus.Error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	name, err := validateName(name)
	if err != nil {
		p.log.Err("AddSystem: %s", err.Error())
		return "", dbus.MakeFailedError(err)
	}
	if _, exists := p.config.Systems[name]; exists {
		err := fmt.Errorf("system already exists: %s", name)
		p.log.Err("AddSystem: %s", err.Error())
		return "", dbus.MakeFailedError(err)
	}

	s := System{Name: name}
	if err := s.DBusRegister(p.conn); err != nil {
		err = fmt.Errorf("failed dbus registration for %s: %v", s.Name, err)
		p.log.Err("AddSystem: %s", err.Error())
		return "", dbus.MakeFailedError(err)
	}
	p.config.Systems[name] = &s
	return SystemObjectPath + dbus.ObjectPath("/"+s.Name), nil
}

func (p *Provisioner) DelSystem(name string) *dbus.Error {
	p.mu.Lock()
	defer p.mu.Unlock()

	name, err := validateName(name)
	if err != nil {
		p.log.Err("DelSystem: %s", err.Error())
		return dbus.MakeFailedError(err)
	}
	s, exists := p.config.Systems[name]
	if !exists {
		err := fmt.Errorf("no such system: %s", name)
		p.log.Err("DelSystem: %s", err.Error())
		return dbus.MakeFailedError(err)
	}
	if err := s.DBusUnregister(p.conn); err != nil {
		p.log.Err("DelSystem: %s", err.Error())
		return dbus.MakeFailedError(err)
	}
	delete(p.config.Systems, name)
	return nil
}

func (p *Provisioner) ListFunctions() ([]string, *dbus.Error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	list := make([]string, 0, len(p.config.Functions))
	for name := range p.config.Functions {
		list = append(list, name)
	}
	return list, nil
}

func (p *Provisioner) AddFunction(name, body string) *dbus.Error {
	p.mu.Lock()
	defer p.mu.Unlock()

	name, err := validateName(name)
	if err != nil {
		p.log.Err("AddFunction: %s", err.Error())
		return dbus.MakeFailedError(err)
	}
	p.config.Functions[name] = &ShellFunc{Name: name, Body: []byte(body)}
	return nil
}

func (p *Provisioner) DelFunction(name string) *dbus.Error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if _, exists := p.config.Functions[name]; !exists {
		err := fmt.Errorf("no such function %s", name)
		p.log.Err("DelFunction: %s", err.Error())
		return dbus.MakeFailedError(err)
	}
	delete(p.config.Functions, name)
	return nil
}

func (p *Provisioner) SyncAnswers() *dbus.Error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.syncAnswers(); err != nil {
		return dbus.MakeFailedError(err)
	}
	if err := p.config.Flush(); err != nil {
		return dbus.MakeFailedError(err)
	}
	return nil
}

func (p *Provisioner) SyncDistros() *dbus.Error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.syncDistros(); err != nil {
		return dbus.MakeFailedError(err)
	}
	if err := p.config.Flush(); err != nil {
		return dbus.MakeFailedError(err)
	}
	return nil
}

func (p *Provisioner) SyncSystems() *dbus.Error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.syncSystems(); err != nil {
		return dbus.MakeFailedError(err)
	}
	if err := p.config.Flush(); err != nil {
		return dbus.MakeFailedError(err)
	}
	return nil
}

func (p *Provisioner) Sync() *dbus.Error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.syncAll(); err != nil {
		return dbus.MakeFailedError(err)
	}
	if err := p.config.Flush(); err != nil {
		return dbus.MakeFailedError(err)
	}
	return nil
}

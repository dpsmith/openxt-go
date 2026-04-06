// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright 2026 Apertus Solutions, LLC
package provisioner

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

var (
	DistrosDir string = "distros"
)

// Distro is an unpacked installer tree named project-build-revision.
// Project may contain hyphens; build and revision are the last two
// hyphen-separated fields.
type Distro struct {
	Name     string
	Project  string
	Build    string
	Revision string
}

// ParseDistroName splits a directory or file name into project, build,
// and revision. It returns nil if the name is not project-build-revision
// or if it contains path / template metacharacters.
func ParseDistroName(name string) []string {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}
	if strings.ContainsAny(name, `/\`) || strings.Contains(name, "..") || strings.Contains(name, "{{") {
		return nil
	}

	revSep := strings.LastIndex(name, "-")
	if revSep <= 0 || revSep == len(name)-1 {
		return nil
	}
	revision := name[revSep+1:]
	rest := name[:revSep]

	buildSep := strings.LastIndex(rest, "-")
	if buildSep <= 0 || buildSep == len(rest)-1 {
		return nil
	}
	build := rest[buildSep+1:]
	project := rest[:buildSep]
	if project == "" || build == "" || revision == "" {
		return nil
	}

	return []string{project, build, revision}
}

func NewDistro(name string) *Distro {
	elems := ParseDistroName(name)
	if elems == nil {
		return nil
	}
	return &Distro{
		Name:     name,
		Project:  elems[0],
		Build:    elems[1],
		Revision: elems[2],
	}
}

func (d *Distro) Version() string {
	if d == nil {
		return ""
	}
	return d.Build + "-" + d.Revision
}

func (d *Distro) FileName() string {
	if d == nil {
		return ""
	}
	if d.Name != "" {
		return d.Name
	}
	return d.Project + "-" + d.Build + "-" + d.Revision
}

func (d *Distro) WriteTftpConfig(root string, t *template.Template) error {
	if d == nil {
		return fmt.Errorf("nil distro")
	}
	if t == nil {
		return fmt.Errorf("nil template provided")
	}
	if ParseDistroName(d.FileName()) == nil {
		return fmt.Errorf("invalid distro name %q", d.FileName())
	}

	contents := struct {
		Name    string
		Version string
	}{
		Name:    d.Project,
		Version: d.Version(),
	}
	if strings.Contains(contents.Name, "{{") || strings.Contains(contents.Version, "{{") {
		return fmt.Errorf("distro %q field contains template metacharacters", d.Name)
	}

	filePath, err := rootedJoin(root, DistrosDir, d.FileName())
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

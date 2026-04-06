// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright 2026 Apertus Solutions, LLC
package provisioner

import (
	"bytes"
	"fmt"
	"strings"
)

// ShellFunc is a named snippet emitted into <preinstall> / <postinstall>.
// Name must be a POSIX-safe identifier; Body is copied verbatim aside
// from a trailing newline.
type ShellFunc struct {
	Name string
	Body []byte
}

func (s *ShellFunc) Marshal() ([]byte, error) {
	if s == nil {
		return nil, fmt.Errorf("nil shell function")
	}
	name, err := validateName(s.Name)
	if err != nil {
		return nil, fmt.Errorf("function name: %v", err)
	}
	if bytes.IndexByte(s.Body, 0) >= 0 {
		return nil, fmt.Errorf("function %s body contains NUL", name)
	}
	if err := checkAnswerTags(name, s.Body); err != nil {
		return nil, err
	}

	body := bytes.TrimRight(s.Body, "\n")
	buf := bytes.Buffer{}
	fmt.Fprintf(&buf, "%s() {\n", name)
	if len(body) > 0 {
		buf.Write(body)
		buf.WriteByte('\n')
	}
	buf.WriteString("}\n")
	return buf.Bytes(), nil
}

type ShellFuncList map[string]*ShellFunc

func (l *ShellFuncList) MarshalList(list []string) ([]byte, error) {
	if l == nil {
		return nil, fmt.Errorf("function list is nil")
	}
	buf := bytes.Buffer{}
	for _, k := range list {
		name, err := validateName(k)
		if err != nil {
			return nil, fmt.Errorf("function name: %v", err)
		}
		f, ok := (*l)[name]
		if !ok {
			return nil, fmt.Errorf("no registered function %s", name)
		}
		m, err := f.Marshal()
		if err != nil {
			return nil, err
		}
		if _, err := buf.Write(m); err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), nil
}

func checkAnswerTags(name string, body []byte) error {
	for _, line := range strings.Split(string(body), "\n") {
		trim := strings.TrimSpace(line)
		if len(trim) >= 3 && trim[0] == '<' && trim[len(trim)-1] == '>' {
			return fmt.Errorf("function %s embeds an answer-file tag: %s", name, trim)
		}
	}
	return nil
}

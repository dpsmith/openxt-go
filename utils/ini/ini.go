/*
 * Code derived from https://github.com/vaughan0/go-ini
 *
 * Copyright (c) 2013 Vaughan Newton
 * Copyright (c) 2026 Daniel P. Smith, Apertus Solutions, LLC
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy of
 * this software and associated documentation files (the "Software"), to deal in
 * the Software without restriction, including without limitation the rights to
 * use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies
 * of the Software, and to permit persons to whom the Software is furnished to do
 * so, subject to the following conditions:
 *
 * The above copyright notice and this permission notice shall be included in all
 * copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 * IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
 * FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
 * AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
 * LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
 * OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
 * SOFTWARE.
 */

// Package ini provides functions for parsing INI configuration files.
package ini

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
)

var (
	sectionRegex = regexp.MustCompile(`^\[(.*)\]$`)
	assignRegex  = regexp.MustCompile(`^([^=]+)=(.*)$`)
)

// ErrSyntax is returned when there is a syntax error in an INI file.
type ErrSyntax struct {
	Line   int
	 // The contents of the erroneous line, without leading or trailing
	 // whitespace
	Source string
}

func (e ErrSyntax) Error() string {
	return fmt.Sprintf("invalid INI syntax on line %d: %s", e.Line, e.Source)
}

var i Ini = make(Ini)

// A Ini represents a parsed INI file.
type Ini map[string]Section

// A Section represents a single section of an INI file.
type Section map[string]string

func GetSection(name string) Section {
	return i.Section(name)
}

// Returns a named Section. A Section will be created if one does not already
// exist for the given name.
func (f Ini) Section(name string) Section {
	section := f[name]
	if section == nil {
		section = make(Section)
		f[name] = section
	}
	return section
}

func Exists(section, key string) bool {
	return i.Exists(section, key)
}

func (f Ini) Exists(section, key string) bool {
	if s := f[section]; s != nil {
		_, ok := s[key]
		return ok
	}
	return false
}

func Get(section, key string) string {
	return i.Get(section, key)
}

// Looks up a value for a key in a section and returns that value, along with a
// boolean result similar to a map lookup.
func (f Ini) Get(section, key string) (value string) {
	if s := f[section]; s != nil {
		value = s[key]
	}
	return
}

func Dump() string {
	return i.Dump()
}

func (f Ini) Dump() string {
	buf := &bytes.Buffer{}

	for n, s := range f {
		fmt.Fprintf(buf, "[%s]\n", n)
		for k, v := range s {
			fmt.Fprintf(buf, "%s = %s\n", k, v)
		}
	}

	return buf.String()
}

func SetDefault(section, key, value string) {
	if !i.Exists(section, key) {
		i.Section(section)[key] = value
	}
}

func parseFile(in *bufio.Reader, ini Ini) (err error) {
	section := ""
	lineNum := 0
	for done := false; !done; {
		var line string
		if line, err = in.ReadString('\n'); err != nil {
			if err == io.EOF {
				done = true
			} else {
				return
			}
		}
		lineNum++
		line = strings.TrimSpace(line)
		if len(line) == 0 {
			// Skip blank lines
			continue
		}
		if line[0] == ';' || line[0] == '#' {
			// Skip comments
			continue
		}

		if line[0] == '[' {
			groups := sectionRegex.FindStringSubmatch(line)
			if groups != nil {
				name := strings.TrimSpace(groups[1])
				section = name
				// Create the section if it does not exist
				ini.Section(section)
			} else {
				return ErrSyntax{lineNum, line}
			}
		} else {
			groups := assignRegex.FindStringSubmatch(line)
			if groups != nil {
				key := strings.TrimSpace(groups[1])
				val := strings.TrimSpace(groups[2])
				ini.Section(section)[key] = val
			} else {
				return ErrSyntax{lineNum, line}
			}
		}

	}
	return nil
}

// Read ini from a reader.
func Read(in io.Reader) error {
	return i.Read(in)
}

// Read ini data from a reader and stores the data in the Ini.
func (f Ini) Read(in io.Reader) error {
	bufin, ok := in.(*bufio.Reader)
	if !ok {
		bufin = bufio.NewReader(in)
	}
	return parseFile(bufin, f)
}

// Read ini in from a file on disk.
func ReadConfig(filename string) error {
	return i.ReadConfig(filename)
}

// Read INI data from a named file and stores the data in the File.
func (f Ini) ReadConfig(file string) error {
	in, err := os.Open(file)
	if err != nil {
		return err
	}
	defer in.Close()
	return f.Read(in)
}

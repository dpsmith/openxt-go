// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright 2026 Apertus Solutions, LLC
//
// Package installer encodes OpenXT installer answer files.
//
// The on-disk format is approximate-XML parsed by a line-oriented matcher,
// not a real XML parser. This package therefore:
//   - emits the exact tag names from answers/doc/answerfiles.txt
//   - keeps every tag except preinstall, postinstall, and quick-option on
//     a single line
//   - rejects '<', '>', quotes in attributes, and unexpected newlines
//     instead of emitting XML entities the installer will not decode
package installer

import (
	"bytes"
	"fmt"
	"strings"
)

// ---------------------------------------------------------------------------
// Attributes
// ---------------------------------------------------------------------------

// Attr is a single tag attribute. Value is formatted with %v; booleans
// become "true"/"false", which matches the installer examples.
type Attr struct {
	Tag   string
	Value interface{}
}

// NewAttr constructs an attribute.
func NewAttr(tag string, value interface{}) *Attr {
	return &Attr{Tag: tag, Value: value}
}

// FindAttr returns the first attribute with the given tag, or nil.
// The slice holds pointers, so the result aliases the caller's value.
func FindAttr(tag string, attrs []*Attr) *Attr {
	for i := range attrs {
		if attrs[i] != nil && attrs[i].Tag == tag {
			return attrs[i]
		}
	}
	return nil
}

func (a *Attr) text() string {
	if a == nil || a.Value == nil {
		return ""
	}
	return fmt.Sprintf("%v", a.Value)
}

func (a *Attr) Marshal() (string, error) {
	if a == nil {
		return "", fmt.Errorf("nil attribute")
	}
	if err := checkName("attribute name", a.Tag); err != nil {
		return "", err
	}
	val := a.text()
	if err := checkAttrValue(a.Tag, val); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s=%q", a.Tag, val), nil
}

// ---------------------------------------------------------------------------
// Tags
// ---------------------------------------------------------------------------

// TagType is an OpenXT answer-file element name.
type TagType string

const (
	InteractiveTag      TagType = "interactive"
	QuickOptionTag      TagType = "quick-option"
	EulaTag             TagType = "eula"
	SourceTag           TagType = "source"
	ModeTag             TagType = "mode"
	PrimaryDiskTag      TagType = "primary-disk"
	PartitionModeTag    TagType = "partition-mode"
	InstallGptTag       TagType = "install-gpt"
	NetworkInterfaceTag TagType = "network-interface"
	LanguageTag         TagType = "language"
	KeyboardTag         TagType = "keyboard"
	PasswordTag         TagType = "password"
	RecoveryPasswordTag TagType = "recovery-password"
	TpmOwnerPasswordTag TagType = "tpm-owner-password"
	EnableSshTag        TagType = "enable-ssh"
	LicenseKeyTag       TagType = "license-key"
	VhdsTag             TagType = "vhds"
	VhdTag              TagType = "vhd"
	VhdSourcesTag       TagType = "vhd-sources"
	VhdSourceTag        TagType = "vhd-source"
	VmsTag              TagType = "vms"
	VmTag               TagType = "vm"
	VmSourceTag         TagType = "vm-source"
	VmVhdsTag           TagType = "vm-vhds"
	VmVhdTag            TagType = "vm-vhd"
	SkipReadyTag        TagType = "skipready"
	PreinstallTag       TagType = "preinstall"
	PostinstallTag      TagType = "postinstall"
	AllowDevRepoCertTag TagType = "allow-dev-repo-cert"
)

var multilineTags = map[TagType]bool{
	QuickOptionTag: true,
	PreinstallTag:  true,
	PostinstallTag: true,
}

// Tag is one answer-file element. Value is one of: nil, string, []byte,
// bool, Tag, or []Tag.
type Tag struct {
	Attrs []*Attr
	Tag   TagType
	Value interface{}
}

// FindTag returns a pointer to the first matching element in tags.
// The pointer aliases tags[i] (same backing array), so assignments
// through it persist. It must not be retained after the slice is
// reallocated.
func FindTag(tag TagType, tags []Tag) *Tag {
	for i := range tags {
		if tags[i].Tag == tag {
			return &tags[i]
		}
	}
	return nil
}

func (t *Tag) multiline() bool {
	return t != nil && multilineTags[t.Tag]
}

func (t *Tag) Marshal() ([]byte, error) {
	if t == nil {
		return nil, fmt.Errorf("nil tag")
	}
	if err := checkName("tag name", string(t.Tag)); err != nil {
		return nil, err
	}

	buf := bytes.Buffer{}
	fmt.Fprintf(&buf, "<%s", t.Tag)
	for _, a := range t.Attrs {
		s, err := a.Marshal()
		if err != nil {
			return nil, fmt.Errorf("tag %s: %v", t.Tag, err)
		}
		fmt.Fprintf(&buf, " %s", s)
	}
	buf.WriteByte('>')

	body, err := marshalValue(t.Tag, t.Value, t.multiline())
	if err != nil {
		return nil, fmt.Errorf("tag %s: %v", t.Tag, err)
	}
	buf.Write(body)
	fmt.Fprintf(&buf, "</%s>", t.Tag)
	return buf.Bytes(), nil
}

func marshalValue(parent TagType, v interface{}, allowNL bool) ([]byte, error) {
	switch val := v.(type) {
	case nil:
		return nil, nil
	case Tag:
		return val.Marshal()
	case []Tag:
		buf := bytes.Buffer{}
		for i := range val {
			b, err := val[i].Marshal()
			if err != nil {
				return nil, err
			}
			buf.Write(b)
		}
		return buf.Bytes(), nil
	case []byte:
		s := string(val)
		if err := checkText(string(parent), s, allowNL); err != nil {
			return nil, err
		}
		return append([]byte(nil), val...), nil
	case string:
		if err := checkText(string(parent), val, allowNL); err != nil {
			return nil, err
		}
		return []byte(val), nil
	case bool:
		if val {
			return []byte("true"), nil
		}
		return []byte("false"), nil
	default:
		s := fmt.Sprintf("%v", val)
		if err := checkText(string(parent), s, allowNL); err != nil {
			return nil, err
		}
		return []byte(s), nil
	}
}

// ---------------------------------------------------------------------------
// Answer file
// ---------------------------------------------------------------------------

// AnswerFile is an ordered list of tags that Marshal encodes to the
// installer answer-file format.
type AnswerFile struct {
	Tags []Tag
}

// NewAnswerFile builds an answer file with the required <source> tag.
// srcType must be one of url, bootmedia, local, harddisk. verify is
// only emitted for type=local; oem is only emitted for type=harddisk.
func NewAnswerFile(srcType, src string, verify, oem bool) *AnswerFile {
	t := Tag{
		Tag:   SourceTag,
		Attrs: []*Attr{NewAttr("type", srcType)},
		Value: src,
	}
	switch srcType {
	case "local":
		t.Attrs = append(t.Attrs, NewAttr("verify", verify))
	case "harddisk":
		t.Attrs = append(t.Attrs, NewAttr("oem", oem))
	}
	return &AnswerFile{Tags: []Tag{t}}
}

func (a *AnswerFile) tagIndex(tag TagType) int {
	if a == nil {
		return -1
	}
	for i := range a.Tags {
		if a.Tags[i].Tag == tag {
			return i
		}
	}
	return -1
}

func (a *AnswerFile) find(tag TagType) *Tag {
	if i := a.tagIndex(tag); i >= 0 {
		return &a.Tags[i]
	}
	return nil
}

func (a *AnswerFile) upsert(t Tag) {
	if i := a.tagIndex(t.Tag); i >= 0 {
		a.Tags[i] = t
		return
	}
	a.Tags = append(a.Tags, t)
}

func (a *AnswerFile) remove(tag TagType) {
	i := a.tagIndex(tag)
	if i < 0 {
		return
	}
	a.Tags = append(a.Tags[:i], a.Tags[i+1:]...)
}

// Marshal encodes the answer file. Source is required. Values that
// would break the installer's line-oriented parser are rejected.
func (a *AnswerFile) Marshal() ([]byte, error) {
	if a == nil {
		return nil, fmt.Errorf("nil answer file")
	}
	if err := a.Validate(); err != nil {
		return nil, err
	}

	buf := bytes.Buffer{}
	for i := range a.Tags {
		b, err := a.Tags[i].Marshal()
		if err != nil {
			return nil, fmt.Errorf("failed marshalling tag %s: %v", a.Tags[i].Tag, err)
		}
		if i != 0 {
			buf.WriteByte('\n')
		}
		buf.Write(b)
	}
	if buf.Len() > 0 {
		buf.WriteByte('\n')
	}
	return buf.Bytes(), nil
}

// Validate reports structural problems that would produce a file the
// installer cannot use. It does not try to decide policy (for example
// whether an unattended install also needs <eula>).
func (a *AnswerFile) Validate() error {
	if a == nil {
		return fmt.Errorf("nil answer file")
	}
	src := a.find(SourceTag)
	if src == nil {
		return fmt.Errorf("missing required source tag")
	}
	typ := ""
	if at := FindAttr("type", src.Attrs); at != nil {
		typ = at.text()
	}
	if err := checkEnum("source type", typ, "url", "bootmedia", "local", "harddisk"); err != nil {
		return err
	}
	return nil
}

// ---------------------------------------------------------------------------
// Setters — keep SetTag as the provisioner-facing entry point
// ---------------------------------------------------------------------------

// SetTag writes or replaces a simple tag. Unknown tags and illegal
// values return an error instead of being coerced to empty.
//
// SkipReadyTag: a true bool (or any other non-false value) emits the
// empty <skipready></skipready> tag; false removes it.
func (a *AnswerFile) SetTag(tag TagType, value interface{}) error {
	if a == nil {
		return fmt.Errorf("nil answer file")
	}
	switch tag {
	case InteractiveTag, EnableSshTag, AllowDevRepoCertTag:
		b, ok := value.(bool)
		if !ok {
			return fmt.Errorf("%s is a bool, gave %T", tag, value)
		}
		a.upsert(Tag{Tag: tag, Value: b})
		return nil

	case ModeTag:
		m, ok := value.(string)
		if !ok {
			return fmt.Errorf("%s is a string, gave %T", tag, value)
		}
		if err := checkEnum(string(tag), m, "fresh", "upgrade"); err != nil {
			return err
		}
		a.upsert(Tag{Tag: tag, Value: m})
		return nil

	case PrimaryDiskTag:
		d, ok := value.(string)
		if !ok {
			return fmt.Errorf("%s is a string, gave %T", tag, value)
		}
		if strings.TrimSpace(d) == "" {
			return fmt.Errorf("%s is empty", tag)
		}
		a.upsert(Tag{Tag: tag, Value: d})
		return nil

	case PartitionModeTag:
		m, ok := value.(string)
		if !ok {
			return fmt.Errorf("%s is a string, gave %T", tag, value)
		}
		if err := checkEnum(string(tag), m,
			"overwrite", "use-free-space", "erase-non-oem", "erase-entire-disk"); err != nil {
			return err
		}
		a.upsert(Tag{Tag: tag, Value: m})
		return nil

	case InstallGptTag:
		g, ok := value.(string)
		if !ok {
			return fmt.Errorf("%s is a string, gave %T", tag, value)
		}
		if err := checkEnum(string(tag), g, "true", "false", "auto"); err != nil {
			return err
		}
		a.upsert(Tag{Tag: tag, Value: g})
		return nil

	case PasswordTag:
		p, ok := value.(string)
		if !ok {
			return fmt.Errorf("%s is a string, gave %T", tag, value)
		}
		if err := checkSystemPassword(p); err != nil {
			return err
		}
		a.upsert(Tag{Tag: tag, Value: p})
		return nil

	case RecoveryPasswordTag, TpmOwnerPasswordTag, LicenseKeyTag:
		p, ok := value.(string)
		if !ok {
			return fmt.Errorf("%s is a string, gave %T", tag, value)
		}
		a.upsert(Tag{Tag: tag, Value: p})
		return nil

	case LanguageTag:
		s, ok := value.(string)
		if !ok {
			return fmt.Errorf("%s is a string, gave %T", tag, value)
		}
		if err := checkEnum(string(tag), s, "en-us"); err != nil {
			return err
		}
		a.upsert(Tag{Tag: tag, Value: s})
		return nil

	case KeyboardTag:
		s, ok := value.(string)
		if !ok {
			return fmt.Errorf("%s is a string, gave %T", tag, value)
		}
		if err := checkEnum(string(tag), s,
			"cn", "fr", "de", "it", "jp", "es", "ch", "gb", "us"); err != nil {
			return err
		}
		a.upsert(Tag{Tag: tag, Value: s})
		return nil

	case SkipReadyTag:
		switch v := value.(type) {
		case bool:
			if !v {
				a.remove(SkipReadyTag)
				return nil
			}
		case string:
			if v == "false" || v == "" {
				a.remove(SkipReadyTag)
				return nil
			}
		}
		a.upsert(Tag{Tag: SkipReadyTag, Value: ""})
		return nil

	case EulaTag:
		s, ok := value.(string)
		if !ok {
			return fmt.Errorf("%s is a string (accept value), gave %T", tag, value)
		}
		return a.SetEula(s)

	default:
		return fmt.Errorf("tag %s is not allowed", tag)
	}
}

// SetEula writes <eula accept="yes|defer"></eula>.
func (a *AnswerFile) SetEula(accept string) error {
	if err := checkEnum("eula accept", accept, "yes", "defer"); err != nil {
		return err
	}
	a.upsert(Tag{
		Tag:   EulaTag,
		Attrs: []*Attr{NewAttr("accept", accept)},
		Value: "",
	})
	return nil
}

// SetPassword writes <password>hash</password>. hash must be empty
// (password-less), "*", or an MD5 crypt string from `openssl passwd -1`.
func (a *AnswerFile) SetPassword(hash string) error {
	return a.SetTag(PasswordTag, hash)
}

// SetPasswordDeferred writes <password defer="true"></password>.
func (a *AnswerFile) SetPasswordDeferred() error {
	a.upsert(Tag{
		Tag:   PasswordTag,
		Attrs: []*Attr{NewAttr("defer", true)},
		Value: "",
	})
	return nil
}

// SetPasswordNone writes <password type="none"></password>, matching
// upstream auto-hard-disk.ans.
func (a *AnswerFile) SetPasswordNone() error {
	a.upsert(Tag{
		Tag:   PasswordTag,
		Attrs: []*Attr{NewAttr("type", "none")},
		Value: "",
	})
	return nil
}

// SetRecoveryPassword writes the plaintext recovery password. The
// installer attempts to scrub this from the file after use; the HTTP
// tree still publishes it until then.
func (a *AnswerFile) SetRecoveryPassword(pw string) error {
	return a.SetTag(RecoveryPasswordTag, pw)
}

// SetTPMOwnerPassword writes the plaintext TPM owner password.
func (a *AnswerFile) SetTPMOwnerPassword(pw string) error {
	return a.SetTag(TpmOwnerPasswordTag, pw)
}

// SetLanguage writes <language>en-us</language>.
func (a *AnswerFile) SetLanguage(code string) error {
	return a.SetTag(LanguageTag, code)
}

// SetLanguageDeferred writes <language defer="true"></language>.
func (a *AnswerFile) SetLanguageDeferred() error {
	a.upsert(Tag{
		Tag:   LanguageTag,
		Attrs: []*Attr{NewAttr("defer", true)},
		Value: "",
	})
	return nil
}

// SetKeyboard writes <keyboard>layout</keyboard>.
func (a *AnswerFile) SetKeyboard(layout string) error {
	return a.SetTag(KeyboardTag, layout)
}

// SetKeyboardDeferred writes <keyboard defer="true"></keyboard>.
func (a *AnswerFile) SetKeyboardDeferred() error {
	a.upsert(Tag{
		Tag:   KeyboardTag,
		Attrs: []*Attr{NewAttr("defer", true)},
		Value: "",
	})
	return nil
}

// SetLicenseKey writes <license-key>…</license-key>.
func (a *AnswerFile) SetLicenseKey(key string) error {
	return a.SetTag(LicenseKeyTag, key)
}

// SetAllowDevRepoCert writes <allow-dev-repo-cert>true|false</allow-dev-repo-cert>.
func (a *AnswerFile) SetAllowDevRepoCert(allow bool) error {
	return a.SetTag(AllowDevRepoCertTag, allow)
}

// SetNetwork writes <network-interface …>. mode=dhcp uses only the
// mode attribute. mode=static requires address, netmask, and gateway;
// dns is optional.
func (a *AnswerFile) SetNetwork(attrs []*Attr) error {
	mode := FindAttr("mode", attrs)
	if mode == nil {
		return fmt.Errorf("no mode provided")
	}
	m := mode.text()
	t := Tag{Tag: NetworkInterfaceTag}

	switch m {
	case "dhcp":
		t.Attrs = []*Attr{NewAttr("mode", "dhcp")}
	case "static":
		addr := FindAttr("address", attrs)
		mask := FindAttr("netmask", attrs)
		gw := FindAttr("gateway", attrs)
		if addr == nil || mask == nil || gw == nil {
			return fmt.Errorf("static mode requires address, netmask, and gateway")
		}
		t.Attrs = []*Attr{
			NewAttr("mode", "static"),
			NewAttr("address", addr.text()),
			NewAttr("netmask", mask.text()),
			NewAttr("gateway", gw.text()),
		}
		if dns := FindAttr("dns", attrs); dns != nil && dns.text() != "" {
			t.Attrs = append(t.Attrs, NewAttr("dns", dns.text()))
		}
	default:
		return fmt.Errorf("invalid mode %s", m)
	}

	a.upsert(t)
	return nil
}

// AddVhd appends a <vhd> under <vhds>. compress may be "", "gzip", or
// "bzip2". At least one source is required.
func (a *AnswerFile) AddVhd(label, compress string, src []string) error {
	if len(src) == 0 {
		return fmt.Errorf("vhd %q has no sources", label)
	}
	if compress != "" && compress != "gzip" && compress != "bzip2" {
		return fmt.Errorf("invalid vhd compress %q", compress)
	}

	sources := make([]Tag, 0, len(src))
	for _, s := range src {
		if strings.TrimSpace(s) == "" {
			return fmt.Errorf("vhd %q has an empty source", label)
		}
		sources = append(sources, Tag{Tag: VhdSourceTag, Value: s})
	}

	vhd := Tag{Tag: VhdTag, Value: Tag{Tag: VhdSourcesTag, Value: sources}}
	if label != "" {
		vhd.Attrs = append(vhd.Attrs, NewAttr("label", label))
	}
	if compress != "" {
		vhd.Attrs = append(vhd.Attrs, NewAttr("compress", compress))
	}

	if existing := a.find(VhdsTag); existing != nil {
		list, ok := existing.Value.([]Tag)
		if !ok {
			return fmt.Errorf("vhds tag value is unexpected, found %T", existing.Value)
		}
		existing.Value = append(list, vhd)
		return nil
	}

	a.Tags = append(a.Tags, Tag{Tag: VhdsTag, Value: []Tag{vhd}})
	return nil
}

// AddVm appends a <vm> under <vms>. vhds are labels matched by
// index (0-based) to entries previously added with AddVhd.
func (a *AnswerFile) AddVm(src string, vhds []string) error {
	if strings.TrimSpace(src) == "" {
		return fmt.Errorf("vm source is empty")
	}

	mapped := make([]Tag, 0, len(vhds))
	for i, label := range vhds {
		mapped = append(mapped, Tag{
			Tag: VmVhdTag,
			Attrs: []*Attr{
				NewAttr("index", i),
				NewAttr("label", label),
			},
		})
	}

	children := []Tag{{Tag: VmSourceTag, Value: src}}
	if len(mapped) > 0 {
		children = append(children, Tag{Tag: VmVhdsTag, Value: mapped})
	}

	vm := Tag{Tag: VmTag, Value: children}

	if existing := a.find(VmsTag); existing != nil {
		list, ok := existing.Value.([]Tag)
		if !ok {
			return fmt.Errorf("vms tag value is unexpected, found %T", existing.Value)
		}
		existing.Value = append(list, vm)
		return nil
	}

	a.Tags = append(a.Tags, Tag{Tag: VmsTag, Value: []Tag{vm}})
	return nil
}

// AddPreInstall sets the <preinstall> body. The installer runs this
// before partitioning. Nested answer-file tags inside the script will
// confuse the parser and are rejected.
func (a *AnswerFile) AddPreInstall(script []byte) error {
	if err := checkScript("preinstall", script); err != nil {
		return err
	}
	a.upsert(Tag{Tag: PreinstallTag, Value: append([]byte(nil), script...)})
	return nil
}

// AddPostInstall sets the <postinstall> body. The installer runs this
// after a successful install.
func (a *AnswerFile) AddPostInstall(script []byte) error {
	if err := checkScript("postinstall", script); err != nil {
		return err
	}
	a.upsert(Tag{Tag: PostinstallTag, Value: append([]byte(nil), script...)})
	return nil
}

// SetQuickOption writes a <quick-option> wrapper around inner tags.
func (a *AnswerFile) SetQuickOption(inner []Tag) error {
	copied := make([]Tag, len(inner))
	copy(copied, inner)
	a.upsert(Tag{Tag: QuickOptionTag, Value: copied})
	return nil
}

// ---------------------------------------------------------------------------
// Validation helpers
// ---------------------------------------------------------------------------

func checkName(kind, name string) error {
	if name == "" {
		return fmt.Errorf("%s is empty", kind)
	}
	for _, r := range name {
		ok := (r >= 'a' && r <= 'z') ||
			(r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') ||
			r == '-'
		if !ok {
			return fmt.Errorf("%s %q is not a valid identifier", kind, name)
		}
	}
	return nil
}

func checkAttrValue(name, val string) error {
	if strings.ContainsRune(val, '"') {
		return fmt.Errorf("attribute %s contains a quote", name)
	}
	if strings.ContainsAny(val, "<>") {
		return fmt.Errorf("attribute %s contains reserved '<' or '>'", name)
	}
	if strings.ContainsAny(val, "\r\n") {
		return fmt.Errorf("attribute %s contains a newline", name)
	}
	return nil
}

func checkText(kind, val string, allowNL bool) error {
	if strings.ContainsAny(val, "<>") {
		return fmt.Errorf("%s contains reserved '<' or '>'", kind)
	}
	if !allowNL && strings.ContainsAny(val, "\r\n") {
		return fmt.Errorf("%s must be a single line", kind)
	}
	return nil
}

func checkScript(kind string, script []byte) error {
	// Scripts may contain newlines and shell metacharacters. They must
	// not include a line that looks like an answer-file tag, which the
	// installer will misread as document structure.
	for _, line := range strings.Split(string(script), "\n") {
		trim := strings.TrimSpace(line)
		if len(trim) >= 3 && trim[0] == '<' && trim[len(trim)-1] == '>' {
			return fmt.Errorf("%s embeds an answer-file tag: %s", kind, trim)
		}
	}
	return nil
}

func checkEnum(kind, val string, allowed ...string) error {
	for _, a := range allowed {
		if val == a {
			return nil
		}
	}
	return fmt.Errorf("invalid value for %s: %q (allowed: %s)",
		kind, val, strings.Join(allowed, ", "))
}

func checkSystemPassword(pw string) error {
	switch {
	case pw == "" || pw == "*":
		return nil
	case strings.HasPrefix(pw, "$1$"):
		if strings.ContainsAny(pw, "<>\"\r\n") {
			return fmt.Errorf("password hash contains reserved characters")
		}
		return nil
	default:
		return fmt.Errorf("password must be empty, \"*\", or an MD5 crypt hash from openssl passwd -1")
	}
}

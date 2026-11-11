package launchers

import (
	"encoding/binary"
	"errors"
	"strings"
	"unicode/utf16"
)

// Shell Link flags we read and write. The rest are preserved bit-for-bit.
const (
	flagHasIDList       uint32 = 0x00000001
	flagHasLinkInfo     uint32 = 0x00000002
	flagHasName         uint32 = 0x00000004
	flagHasRelativePath uint32 = 0x00000008
	flagHasWorkingDir   uint32 = 0x00000010
	flagHasArguments    uint32 = 0x00000020
	flagHasIconLocation uint32 = 0x00000040
	flagIsUnicode       uint32 = 0x00000080
)

var linkCLSID = []byte{
	0x01, 0x14, 0x02, 0x00, 0x00, 0x00, 0x00, 0x00,
	0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46,
}

// Shortcut is the part of a .lnk file IconCraft understands. Raw sections
// (the item-id list, link info and extra data) are kept so changing the icon
// does not rebuild, and therefore cannot corrupt, the target.
type Shortcut struct {
	header    []byte
	flags     uint32
	IconIndex int32
	Target    string
	Name      string
	Relative  string
	WorkDir   string
	Args      string
	Icon      string
	idList    []byte
	linkInfo  []byte
	extra     []byte
}

// ReadShortcut parses a Shell Link. It refuses a file it cannot account for
// completely, because a partial write would damage a launcher the user still
// needs.
func ReadShortcut(b []byte) (Shortcut, error) {
	if len(b) < 0x4C {
		return Shortcut{}, errors.New("shortcut is too small")
	}
	headerSize := binary.LittleEndian.Uint32(b[0:4])
	if headerSize < 0x4C || int(headerSize) > len(b) {
		return Shortcut{}, errors.New("shortcut header is the wrong size")
	}
	if !bytesEqual(b[4:20], linkCLSID) {
		return Shortcut{}, errors.New("shortcut is not a Shell Link")
	}
	s := Shortcut{
		header:    append([]byte{}, b[:headerSize]...),
		flags:     binary.LittleEndian.Uint32(b[0x14:0x18]),
		IconIndex: int32(binary.LittleEndian.Uint32(b[0x38:0x3C])),
	}
	p := int(headerSize)
	if s.flags&flagHasIDList != 0 {
		if p+2 > len(b) {
			return Shortcut{}, errors.New("shortcut item list is truncated")
		}
		n := int(binary.LittleEndian.Uint16(b[p : p+2]))
		// IDListSize is the size of the list that follows, not including itself.
		if n < 2 || p+2+n > len(b) {
			return Shortcut{}, errors.New("shortcut item list is truncated")
		}
		s.idList = append([]byte{}, b[p:p+2+n]...)
		p += 2 + n
	}
	if s.flags&flagHasLinkInfo != 0 {
		if p+4 > len(b) {
			return Shortcut{}, errors.New("shortcut link info is truncated")
		}
		n := int(binary.LittleEndian.Uint32(b[p : p+4]))
		if n < 0x1C || p+n > len(b) {
			return Shortcut{}, errors.New("shortcut link info is truncated")
		}
		s.linkInfo = append([]byte{}, b[p:p+n]...)
		s.Target = linkTarget(s.linkInfo)
		p += n
	}
	var err error
	read := func(flag uint32, dst *string) error {
		if s.flags&flag == 0 {
			return nil
		}
		v, n, err := readCounted(b[p:], s.flags&flagIsUnicode != 0)
		if err != nil {
			return err
		}
		*dst = v
		p += n
		return nil
	}
	if err = read(flagHasName, &s.Name); err != nil {
		return Shortcut{}, err
	}
	if err = read(flagHasRelativePath, &s.Relative); err != nil {
		return Shortcut{}, err
	}
	if err = read(flagHasWorkingDir, &s.WorkDir); err != nil {
		return Shortcut{}, err
	}
	if err = read(flagHasArguments, &s.Args); err != nil {
		return Shortcut{}, err
	}
	if err = read(flagHasIconLocation, &s.Icon); err != nil {
		return Shortcut{}, err
	}
	if s.Target == "" {
		s.Target = envTarget(b[p:])
	}
	s.extra = append([]byte{}, b[p:]...)
	return s, nil
}

// SetIcon points the shortcut at an icon file. The target is untouched.
func (s *Shortcut) SetIcon(path string, index int32) {
	s.Icon = path
	s.IconIndex = index
	s.flags |= flagHasIconLocation | flagIsUnicode
}

// ClearIcon restores a shortcut that originally had no custom icon.
func (s *Shortcut) ClearIcon() {
	s.Icon = ""
	s.IconIndex = 0
	s.flags &^= flagHasIconLocation
}

// Bytes serialises the shortcut. The item-id list, link info and extra data
// are the original bytes.
func (s Shortcut) Bytes() ([]byte, error) {
	if len(s.header) < 0x4C {
		return nil, errors.New("shortcut header is missing")
	}
	header := append([]byte{}, s.header...)
	binary.LittleEndian.PutUint32(header[0x14:0x18], s.flags)
	binary.LittleEndian.PutUint32(header[0x38:0x3C], uint32(s.IconIndex))
	out := header
	out = append(out, s.idList...)
	out = append(out, s.linkInfo...)
	unicode := s.flags&flagIsUnicode != 0
	write := func(flag uint32, v string) {
		if s.flags&flag == 0 {
			return
		}
		out = append(out, counted(v, unicode)...)
	}
	write(flagHasName, s.Name)
	write(flagHasRelativePath, s.Relative)
	write(flagHasWorkingDir, s.WorkDir)
	write(flagHasArguments, s.Args)
	write(flagHasIconLocation, s.Icon)
	extra := s.extra
	if len(extra) == 0 || binary.LittleEndian.Uint32(pad4(extra)) != 0 && !hasTerminal(extra) {
		extra = append(append([]byte{}, extra...), 0, 0, 0, 0)
	}
	out = append(out, extra...)
	return out, nil
}

func hasTerminal(b []byte) bool {
	if len(b) < 4 {
		return false
	}
	n := binary.LittleEndian.Uint32(b[len(b)-4:])
	return n < 0x4
}

func pad4(b []byte) []byte {
	if len(b) >= 4 {
		return b[len(b)-4:]
	}
	return []byte{0, 0, 0, 0}
}

// NewShortcut builds a minimal launcher for tests and for a personal copy of
// a shortcut IconCraft is not allowed to edit in place. Real shortcuts that
// already exist are updated with ReadShortcut, which keeps their target data.
func NewShortcut(target string) Shortcut {
	header := make([]byte, 0x4C)
	binary.LittleEndian.PutUint32(header[0:4], 0x4C)
	copy(header[4:20], linkCLSID)
	flags := flagHasLinkInfo | flagIsUnicode
	binary.LittleEndian.PutUint32(header[0x14:0x18], flags)
	binary.LittleEndian.PutUint32(header[0x18:0x1C], 0x20) // archive
	binary.LittleEndian.PutUint32(header[0x3C:0x40], 1)    // SW_SHOWNORMAL
	return Shortcut{
		header:   header,
		flags:    flags,
		Target:   target,
		linkInfo: buildLinkInfo(target),
		extra:    []byte{0, 0, 0, 0},
	}
}

func buildLinkInfo(target string) []byte {
	// Header size 0x1C, VolumeIDAndLocalBasePath, an empty volume label and
	// an ANSI local path. ASCII targets are what the spec's own example uses;
	// non-ASCII targets are still stored, as Windows-1252-or-UTF8 bytes plus
	// the original file's link info when we are editing a real shortcut.
	path := append([]byte(target), 0)
	const header = 0x1C
	volSize := 0x11
	localOff := header + volSize
	suffixOff := localOff + len(path)
	total := suffixOff + 1
	b := make([]byte, total)
	binary.LittleEndian.PutUint32(b[0:4], uint32(total))
	binary.LittleEndian.PutUint32(b[4:8], header)
	binary.LittleEndian.PutUint32(b[8:12], 1) // VolumeIDAndLocalBasePath
	binary.LittleEndian.PutUint32(b[12:16], header)
	binary.LittleEndian.PutUint32(b[16:20], uint32(localOff))
	binary.LittleEndian.PutUint32(b[20:24], 0)
	binary.LittleEndian.PutUint32(b[24:28], uint32(suffixOff))
	binary.LittleEndian.PutUint32(b[header:header+4], uint32(volSize))
	binary.LittleEndian.PutUint32(b[header+4:header+8], 3) // DRIVE_FIXED
	binary.LittleEndian.PutUint32(b[header+12:header+16], 0x10)
	copy(b[localOff:], path)
	return b
}

func linkTarget(b []byte) string {
	if len(b) < 0x1C {
		return ""
	}
	headerSize := binary.LittleEndian.Uint32(b[4:8])
	flags := binary.LittleEndian.Uint32(b[8:12])
	if flags&1 == 0 || headerSize < 0x1C || int(headerSize) > len(b) {
		return ""
	}
	localOff := binary.LittleEndian.Uint32(b[0x10:0x14])
	suffixOff := binary.LittleEndian.Uint32(b[0x18:0x1C])
	base := cstr(b, localOff)
	suffix := cstr(b, suffixOff)
	if headerSize >= 0x24 && len(b) >= 0x24 {
		if off := binary.LittleEndian.Uint32(b[0x1C:0x20]); off != 0 {
			if s := utf16z(b, off); s != "" {
				base = s
			}
		}
		if off := binary.LittleEndian.Uint32(b[0x20:0x24]); off != 0 {
			if s := utf16z(b, off); s != "" {
				suffix = s
			}
		}
	}
	switch {
	case base == "":
		return suffix
	case suffix == "":
		return base
	case strings.HasSuffix(base, `\`) || strings.HasSuffix(base, `/`):
		return base + suffix
	default:
		return base + `\` + suffix
	}
}

func cstr(b []byte, off uint32) string {
	if int(off) >= len(b) {
		return ""
	}
	rest := b[off:]
	i := 0
	for i < len(rest) && rest[i] != 0 {
		i++
	}
	return string(rest[:i])
}

func utf16z(b []byte, off uint32) string {
	if int(off) >= len(b) {
		return ""
	}
	rest := b[off:]
	var u []uint16
	for i := 0; i+1 < len(rest); i += 2 {
		c := uint16(rest[i]) | uint16(rest[i+1])<<8
		if c == 0 {
			break
		}
		u = append(u, c)
	}
	return string(utf16.Decode(u))
}

func readCounted(b []byte, unicode bool) (string, int, error) {
	if len(b) < 2 {
		return "", 0, errors.New("shortcut string is truncated")
	}
	n := int(binary.LittleEndian.Uint16(b[:2]))
	if unicode {
		size := 2 + n*2
		if n > 32*1024 || size > len(b) {
			return "", 0, errors.New("shortcut string is truncated")
		}
		return decodeUTF16(b[2:size]), size, nil
	}
	size := 2 + n
	if n > 32*1024 || size > len(b) {
		return "", 0, errors.New("shortcut string is truncated")
	}
	return string(b[2:size]), size, nil
}

func counted(s string, unicode bool) []byte {
	if unicode {
		raw := encodeUTF16(s)
		n := len(raw) / 2
		out := make([]byte, 2+len(raw))
		binary.LittleEndian.PutUint16(out[:2], uint16(n))
		copy(out[2:], raw)
		return out
	}
	out := make([]byte, 2+len(s))
	binary.LittleEndian.PutUint16(out[:2], uint16(len(s)))
	copy(out[2:], s)
	return out
}

// envTarget reads an EnvironmentVariableDataBlock when the link info has no
// path of its own. The block signature is 0xA0000001.
func envTarget(extra []byte) string {
	for len(extra) >= 8 {
		n := binary.LittleEndian.Uint32(extra[:4])
		if n < 0x4 {
			return ""
		}
		if int(n) > len(extra) || n > 1<<20 {
			return ""
		}
		sig := binary.LittleEndian.Uint32(extra[4:8])
		if sig == 0xA0000001 && n >= 8+260+520 {
			if s := strings.TrimRight(utf16z(extra[8+260:], 0), "\x00"); s != "" {
				return s
			}
			return cstr(extra[8:], 0)
		}
		extra = extra[n:]
	}
	return ""
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

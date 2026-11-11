package launchers

import (
	"bytes"
	"encoding/hex"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
)

func mustPNG(t *testing.T, n int) []byte {
	t.Helper()
	im := image.NewNRGBA(image.Rect(0, 0, n, n))
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			im.SetNRGBA(x, y, color.NRGBA{R: 20, G: 80, B: 200, A: 255})
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, im); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// Published in MS-SHLLINK 3.1 "Shortcut to a File". The target is C:\test\a.txt.
const sampleLNK = "" +
	"4c0000000114020000000000c0000000000000469b00080020000000d0e9eef21515c901" +
	"d0e9eef21515c901d0e9eef21515c9010000000000000000010000000000000000000000" +
	"00000000bd0014001f50e04fd020ea3a6910a2d808002b30309d19002f433a5c00000000" +
	"00000000000000000000000000000046003100000000002c3969a3100074657374000032" +
	"0007000400efbe2c3965a32c3969a326000000031e00000000f51e000000000000000000" +
	"0074006500730074000000140048003200000000002c3969a32000612e74787400340007" +
	"000400efbe2c3969a32c3969a3260000002d6e0000000096010000000000000000000061" +
	"002e007400780074000000140000003c0000001c000000010000001c0000002d00000000" +
	"0000003b0000001100000003000000818a7a301000000000433a5c746573745c612e7478" +
	"74000007002e005c0061002e00740078007400070043003a005c00740065007300740060" +
	"000000030000a0580000000000000063687269732d787073000000000000004078c79447" +
	"fac746b3565c2dc6b6d115ec46cd7b227fdd11949900137216874a4078c79447fac746b3" +
	"565c2dc6b6d115ec46cd7b227fdd11949900137216874a00000000"

func TestSampleShortcutTargetSurvivesIconEdit(t *testing.T) {
	raw, err := hex.DecodeString(sampleLNK)
	if err != nil {
		t.Fatal(err)
	}
	s, err := ReadShortcut(raw)
	if err != nil {
		t.Fatal(err)
	}
	if s.Target != `C:\test\a.txt` {
		t.Fatalf("target %q", s.Target)
	}
	if s.Relative != `.\a.txt` || !strings.EqualFold(s.WorkDir, `c:\test`) {
		t.Fatalf("relative %q work %q", s.Relative, s.WorkDir)
	}
	s.SetIcon(`C:\Users\me\AppData\Roaming\IconCraft\launchers\demo.ico`, 0)
	out, err := s.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	again, err := ReadShortcut(out)
	if err != nil {
		t.Fatal(err)
	}
	if again.Target != `C:\test\a.txt` {
		t.Fatalf("target changed to %q", again.Target)
	}
	if again.Icon != `C:\Users\me\AppData\Roaming\IconCraft\launchers\demo.ico` || again.IconIndex != 0 {
		t.Fatalf("icon %q index %d", again.Icon, again.IconIndex)
	}
	again.ClearIcon()
	cleared, err := again.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	back, err := ReadShortcut(cleared)
	if err != nil {
		t.Fatal(err)
	}
	if back.Icon != "" || back.Target != `C:\test\a.txt` {
		t.Fatalf("cleared icon %q target %q", back.Icon, back.Target)
	}
}

func TestNewShortcutRoundTrip(t *testing.T) {
	s := NewShortcut(`C:\Program Files\PhotoCraft\PhotoCraft.exe`)
	s.SetIcon(`D:\icons\photo.ico`, 0)
	b, err := s.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	got, err := ReadShortcut(b)
	if err != nil {
		t.Fatal(err)
	}
	if got.Target != `C:\Program Files\PhotoCraft\PhotoCraft.exe` || got.Icon != `D:\icons\photo.ico` {
		t.Fatalf("%+v", got)
	}
}

func TestICORoundTripPicksLargestPNG(t *testing.T) {
	png := mustPNG(t, 64)
	ico, err := EncodeICO(png)
	if err != nil {
		t.Fatal(err)
	}
	back, err := ToPNG(ico)
	if err != nil {
		t.Fatal(err)
	}
	if len(back) < 24 {
		t.Fatal("empty png")
	}
}

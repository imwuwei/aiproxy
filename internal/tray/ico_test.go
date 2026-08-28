package tray

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"testing"
)

// testPNG 构造一张 100×100 的测试 PNG（与 assets/aiproxy.png 同尺寸规格）。
func testPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			img.SetRGBA(x, y, color.RGBA{uint8(x * 2), uint8(y * 2), 128, 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode test png: %v", err)
	}
	return buf.Bytes()
}

func TestPNGToICO(t *testing.T) {
	icoBytes, err := pngToICO(testPNG(t))
	if err != nil {
		t.Fatalf("pngToICO: %v", err)
	}

	// ICONDIR：reserved(2) + type(2) + count(2)
	if len(icoBytes) < 6 {
		t.Fatalf("ico too short: %d bytes", len(icoBytes))
	}
	if binary.LittleEndian.Uint16(icoBytes[0:2]) != 0 {
		t.Error("reserved field must be 0")
	}
	if binary.LittleEndian.Uint16(icoBytes[2:4]) != 1 {
		t.Error("type field must be 1 (icon)")
	}
	count := int(binary.LittleEndian.Uint16(icoBytes[4:6]))
	if count != len(icoSizes) {
		t.Fatalf("entry count = %d, want %d", count, len(icoSizes))
	}

	seen := map[int]bool{}
	for i := 0; i < count; i++ {
		e := 6 + 16*i
		if e+16 > len(icoBytes) {
			t.Fatalf("entry %d header out of range", i)
		}
		w := int(icoBytes[e])
		h := int(icoBytes[e+1])
		dataLen := int(binary.LittleEndian.Uint32(icoBytes[e+8 : e+12]))
		offset := int(binary.LittleEndian.Uint32(icoBytes[e+12 : e+16]))
		if offset+dataLen > len(icoBytes) {
			t.Fatalf("entry %d image data out of range", i)
		}
		// 每帧必须是可解码的 PNG，且尺寸与条目声明一致
		dec, err := png.Decode(bytes.NewReader(icoBytes[offset : offset+dataLen]))
		if err != nil {
			t.Fatalf("entry %d (%dx%d) is not a valid PNG: %v", i, w, h, err)
		}
		if dec.Bounds().Dx() != w || dec.Bounds().Dy() != h {
			t.Errorf("entry %d: PNG %dx%d != declared %dx%d",
				i, dec.Bounds().Dx(), dec.Bounds().Dy(), w, h)
		}
		seen[w] = true
	}
	for _, s := range icoSizes {
		if !seen[s] {
			t.Errorf("missing size %d in ICO", s)
		}
	}
}

func TestResizeRGBA(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 100, 100))
	// 左上 50×50 画纯色，其余为透明
	for y := 0; y < 50; y++ {
		for x := 0; x < 50; x++ {
			src.SetRGBA(x, y, color.RGBA{200, 100, 50, 255})
		}
	}
	dst := resizeRGBA(src, 16, 16)
	if dst.Bounds().Dx() != 16 || dst.Bounds().Dy() != 16 {
		t.Fatalf("resize size = %v, want 16×16", dst.Bounds())
	}
	// 目标左上角像素（对应源左上纯色区域）应为纯色
	c := dst.RGBAAt(0, 0)
	if c.R != 200 || c.G != 100 || c.B != 50 || c.A != 255 {
		t.Errorf("top-left pixel = %v, want (200,100,50,255)", c)
	}
	// 目标右下角像素（对应源右下透明区域）应全透明
	if c = dst.RGBAAt(15, 15); c.A != 0 {
		t.Errorf("bottom-right alpha = %d, want 0", c.A)
	}
}

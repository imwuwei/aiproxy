package tray

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
)

// icoSizes 托盘 ICO 内包含的尺寸（小图标 16、中 24、标准 32、高分屏 48）。
var icoSizes = []int{16, 24, 32, 48}

// pngToICO 将 PNG 图标字节转换为多尺寸 ICO 文件字节。
// ICO 内每个条目使用 PNG 压缩图像（Windows Vista 及以后支持），可直接写入文件后由 LoadImageW 加载。
func pngToICO(pngBytes []byte) ([]byte, error) {
	// image.Decode 依赖 image/png 的 init 注册，可解码本包传入的 PNG。
	src, _, err := image.Decode(bytes.NewReader(pngBytes))
	if err != nil {
		return nil, err
	}

	type frame struct {
		w, h int
		data []byte
	}
	frames := make([]frame, 0, len(icoSizes))
	for _, s := range icoSizes {
		img := resizeRGBA(src, s, s)
		var buf bytes.Buffer
		if err := png.Encode(&buf, img); err != nil {
			return nil, err
		}
		frames = append(frames, frame{s, s, buf.Bytes()})
	}

	var out bytes.Buffer
	// ICONDIR：reserved(2) / type(2) / count(2)
	binary.Write(&out, binary.LittleEndian, uint16(0))
	binary.Write(&out, binary.LittleEndian, uint16(1)) // type = icon
	binary.Write(&out, binary.LittleEndian, uint16(len(frames)))

	offset := 6 + 16*len(frames)
	for _, f := range frames {
		// ICONDIRENTRY（16 字节）
		out.WriteByte(byte(f.w)) // 0 表示 256，此处尺寸均 < 256
		out.WriteByte(byte(f.h))
		out.WriteByte(0)                                    // 调色板数，不使用
		out.WriteByte(0)                                    // reserved
		binary.Write(&out, binary.LittleEndian, uint16(1))  // planes
		binary.Write(&out, binary.LittleEndian, uint16(32)) // bpp
		binary.Write(&out, binary.LittleEndian, uint32(len(f.data)))
		binary.Write(&out, binary.LittleEndian, uint32(offset))
		offset += len(f.data)
	}
	for _, f := range frames {
		out.Write(f.data)
	}
	return out.Bytes(), nil
}

// resizeRGBA 将 src 面积平均（box filter）缩放到 w×h，适合缩小场景
// （如 100×100 → 16/32/48），并正确处理 alpha 预乘。
func resizeRGBA(src image.Image, w, h int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	sw, sh := src.Bounds().Dx(), src.Bounds().Dy()

	// 目标像素 (x,y) 对应源像素区域 [x0,x1) × [y0,y1)
	for y := 0; y < h; y++ {
		y0 := y * sh / h
		y1 := (y + 1) * sh / h
		if y1 <= y0 {
			y1 = y0 + 1
		}
		for x := 0; x < w; x++ {
			x0 := x * sw / w
			x1 := (x + 1) * sw / w
			if x1 <= x0 {
				x1 = x0 + 1
			}

			var rSum, gSum, bSum, aSum uint32
			var n uint32
			for sy := y0; sy < y1; sy++ {
				for sx := x0; sx < x1; sx++ {
					// RGBA() 返回 16bit 预乘值，取高 8 位参与平均
					r, g, b, a := src.At(sx, sy).RGBA()
					rSum += r >> 8
					gSum += g >> 8
					bSum += b >> 8
					aSum += a >> 8
					n++
				}
			}

			aAvg := aSum / n
			if aAvg == 0 {
				dst.SetRGBA(x, y, color.RGBA{})
				continue
			}
			// 平均预乘色 → 非预乘（color.RGBA 按非预乘存储）。
			// 因逐像素 r<=a，故 rSum/n <= aAvg，还原结果不会超过 255。
			r := uint8(rSum / n * 255 / aAvg)
			g := uint8(gSum / n * 255 / aAvg)
			b := uint8(bSum / n * 255 / aAvg)
			dst.SetRGBA(x, y, color.RGBA{r, g, b, uint8(aAvg)})
		}
	}
	return dst
}

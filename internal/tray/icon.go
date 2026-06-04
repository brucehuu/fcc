//go:build darwin

package tray

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
)

const (
	iconPath    = "assets/fcc-logo.png"
	cornerRatio = 23 // % of width used as corner radius — larger for Dock visibility
)

// loadIcon returns PNG bytes for the menu bar icon. If the user has dropped
// a real logo at assets/fcc-logo.png, we apply rounded corners to it;
// otherwise we return a generated black-on-transparent circle as a placeholder.
func loadIcon() []byte {
	data, err := os.ReadFile(iconPath)
	if err != nil || len(data) == 0 {
		return placeholderIcon()
	}
	if rounded, ok := ApplyRoundedCorners(data); ok {
		return rounded
	}
	return data
}

func placeholderIcon() []byte {
	const size = 22
	img := image.NewRGBA(image.Rect(0, 0, size, size))

	black := color.RGBA{0, 0, 0, 255}
	clear := color.RGBA{0, 0, 0, 0}

	center := size / 2
	radius := size/2 - 1
	r2 := radius * radius

	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			dx := x - center
			dy := y - center
			if dx*dx+dy*dy <= r2 {
				img.Set(x, y, black)
			} else {
				img.Set(x, y, clear)
			}
		}
	}

	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

// ApplyRoundedCorners 读取 PNG 字节、给四个角裁圆，再编码回 PNG。
// 解码/编码失败时返回 (nil, false)，调用方应回退到原图。
func ApplyRoundedCorners(pngBytes []byte) ([]byte, bool) {
	src, _, err := image.Decode(bytes.NewReader(pngBytes))
	if err != nil {
		return nil, false
	}

	bounds := src.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if w <= 0 || h <= 0 {
		return nil, false
	}
	radius := w * cornerRatio / 100
	if radius > h/2 {
		radius = h / 2
	}

	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r, g, b, a := src.At(x, y).RGBA()
			if !insideRoundedRect(x, y, w, h, radius) {
				a = 0
			}
			dst.SetRGBA(x, y, color.RGBA{
				R: uint8(r >> 8),
				G: uint8(g >> 8),
				B: uint8(b >> 8),
				A: uint8(a >> 8),
			})
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, dst); err != nil {
		return nil, false
	}
	return buf.Bytes(), true
}

// RemoveWhiteBackground 把接近白色的背景像素变为透明（alpha=0）。
// 原图是 RGB 无 alpha，Dock 里白色背景会把圆角盖住，必须先抠掉白底。
func RemoveWhiteBackground(pngBytes []byte) ([]byte, bool) {
	src, _, err := image.Decode(bytes.NewReader(pngBytes))
	if err != nil {
		return nil, false
	}
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return nil, false
	}

	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r, g, bch, a := src.At(x, y).RGBA()
			r8, g8, b8, a8 := uint8(r>>8), uint8(g>>8), uint8(bch>>8), uint8(a>>8)
			// 接近白色的像素（阈值 240/255）设为透明
			if r8 > 240 && g8 > 240 && b8 > 240 {
				a8 = 0
			}
			dst.SetRGBA(x, y, color.RGBA{R: r8, G: g8, B: b8, A: a8})
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, dst); err != nil {
		return nil, false
	}
	return buf.Bytes(), true
}

// AddIconPadding 读取 PNG 字节，四周各扩展 paddingPercent% 的空白区域，
// 再把原图居中贴上去。macOS Dock 图标通常内容只占 ~75-80%，
// 不加 padding 会显得比别的图标大一圈。
func AddIconPadding(pngBytes []byte, paddingPercent int) ([]byte, bool) {
	src, _, err := image.Decode(bytes.NewReader(pngBytes))
	if err != nil {
		return nil, false
	}

	b := src.Bounds()
	ow, oh := b.Dx(), b.Dy()
	if ow <= 0 || oh <= 0 {
		return nil, false
	}

	// 新画布 = 原图 + 四周 padding
	padW := ow * paddingPercent / 100
	padH := oh * paddingPercent / 100
	w, h := ow+padW*2, oh+padH*2

	dst := image.NewRGBA(image.Rect(0, 0, w, h))

	// 居中贴原图
	offX, offY := padW, padH
	for y := 0; y < oh; y++ {
		for x := 0; x < ow; x++ {
			dst.Set(offX+x, offY+y, src.At(b.Min.X+x, b.Min.Y+y))
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, dst); err != nil {
		return nil, false
	}
	return buf.Bytes(), true
}

// insideRoundedRect 判断像素 (x, y) 是否在宽 w 高 h、半径 r 的圆角矩形内。
// 边和中心区域直接命中，四个角做圆形裁切。
func insideRoundedRect(x, y, w, h, r int) bool {
	switch {
	case x < r && y < r:
		dx, dy := r-x, r-y
		return dx*dx+dy*dy <= r*r
	case x >= w-r && y < r:
		dx, dy := x-(w-r-1), r-y
		return dx*dx+dy*dy <= r*r
	case x < r && y >= h-r:
		dx, dy := r-x, y-(h-r-1)
		return dx*dx+dy*dy <= r*r
	case x >= w-r && y >= h-r:
		dx, dy := x-(w-r-1), y-(h-r-1)
		return dx*dx+dy*dy <= r*r
	default:
		return true
	}
}

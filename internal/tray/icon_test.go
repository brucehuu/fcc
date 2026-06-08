package tray

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func TestLoadIcon(t *testing.T) {
	// assets/fcc-logo.png likely doesn't exist in test environment,
	// so loadIcon should fall back to placeholderIcon.
	data := loadIcon()
	if len(data) == 0 {
		t.Fatal("loadIcon() returned empty data")
	}
	// Verify it's valid PNG.
	_, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Decode error = %v", err)
	}
	if format != "png" {
		t.Errorf("format = %q, want png", format)
	}
}

func TestPlaceholderIcon(t *testing.T) {
	data := placeholderIcon()
	if len(data) == 0 {
		t.Fatal("placeholderIcon() returned empty data")
	}

	// Verify it's valid PNG.
	img, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Decode error = %v", err)
	}
	if format != "png" {
		t.Errorf("format = %q, want png", format)
	}
	if img.Bounds().Dx() != 22 || img.Bounds().Dy() != 22 {
		t.Errorf("size = %dx%d, want 22x22", img.Bounds().Dx(), img.Bounds().Dy())
	}
}

func TestApplyRoundedCorners(t *testing.T) {
	// Create a simple 100x100 red PNG.
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			img.Set(x, y, color.RGBA{255, 0, 0, 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("Encode error = %v", err)
	}

	rounded, ok := ApplyRoundedCorners(buf.Bytes())
	if !ok {
		t.Fatal("ApplyRoundedCorners() failed")
	}

	// Decode and check corners are transparent.
	decoded, _, err := image.Decode(bytes.NewReader(rounded))
	if err != nil {
		t.Fatalf("Decode error = %v", err)
	}

	// Top-left corner should be transparent.
	_, _, _, a := decoded.At(0, 0).RGBA()
	if a != 0 {
		t.Error("top-left corner should be transparent")
	}

	// Center should still be red.
	r, g, b, a := decoded.At(50, 50).RGBA()
	if r == 0 || g != 0 || b != 0 || a == 0 {
		t.Errorf("center pixel = (%d,%d,%d,%d), want red opaque", r, g, b, a)
	}
}

func TestApplyRoundedCornersInvalid(t *testing.T) {
	_, ok := ApplyRoundedCorners([]byte("not a png"))
	if ok {
		t.Error("ApplyRoundedCorners() should fail for invalid data")
	}
}

func TestRemoveWhiteBackground(t *testing.T) {
	// Create a 10x10 image: left half white, right half red.
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	for y := 0; y < 10; y++ {
		for x := 0; x < 5; x++ {
			img.Set(x, y, color.RGBA{255, 255, 255, 255})
		}
		for x := 5; x < 10; x++ {
			img.Set(x, y, color.RGBA{255, 0, 0, 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("Encode error = %v", err)
	}

	result, ok := RemoveWhiteBackground(buf.Bytes())
	if !ok {
		t.Fatal("RemoveWhiteBackground() failed")
	}

	decoded, _, err := image.Decode(bytes.NewReader(result))
	if err != nil {
		t.Fatalf("Decode error = %v", err)
	}

	// Left half (white) should be transparent.
	_, _, _, a := decoded.At(0, 0).RGBA()
	if a != 0 {
		t.Error("white pixel should be transparent")
	}

	// Right half (red) should remain opaque.
	_, _, _, a = decoded.At(7, 7).RGBA()
	if a == 0 {
		t.Error("red pixel should remain opaque")
	}
}

func TestRemoveWhiteBackgroundInvalid(t *testing.T) {
	_, ok := RemoveWhiteBackground([]byte("not a png"))
	if ok {
		t.Error("RemoveWhiteBackground() should fail for invalid data")
	}
}

func TestAddIconPadding(t *testing.T) {
	// Create a 10x10 red PNG.
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	for y := 0; y < 10; y++ {
		for x := 0; x < 10; x++ {
			img.Set(x, y, color.RGBA{255, 0, 0, 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("Encode error = %v", err)
	}

	result, ok := AddIconPadding(buf.Bytes(), 20)
	if !ok {
		t.Fatal("AddIconPadding() failed")
	}

	decoded, _, err := image.Decode(bytes.NewReader(result))
	if err != nil {
		t.Fatalf("Decode error = %v", err)
	}

	// Original 10x10 with 20% padding on each side = 10 + 2*2 = 14x14.
	if decoded.Bounds().Dx() != 14 || decoded.Bounds().Dy() != 14 {
		t.Errorf("size = %dx%d, want 14x14", decoded.Bounds().Dx(), decoded.Bounds().Dy())
	}

	// Center should be red.
	r, g, b, a := decoded.At(7, 7).RGBA()
	if r == 0 || g != 0 || b != 0 || a == 0 {
		t.Errorf("center pixel = (%d,%d,%d,%d), want red opaque", r, g, b, a)
	}
}

func TestAddIconPaddingInvalid(t *testing.T) {
	_, ok := AddIconPadding([]byte("not a png"), 10)
	if ok {
		t.Error("AddIconPadding() should fail for invalid data")
	}
}

func TestInsideRoundedRect(t *testing.T) {
	tests := []struct {
		x, y, w, h, r int
		want          bool
	}{
		// Center.
		{50, 50, 100, 100, 10, true},
		// Top-left corner (inside).
		{5, 5, 100, 100, 10, true},
		// Top-left corner (outside).
		{2, 2, 100, 100, 10, false},
		// Top-right corner (inside).
		{95, 5, 100, 100, 10, true},
		// Top-right corner (outside).
		{98, 2, 100, 100, 10, false},
		// Bottom-left corner (inside).
		{5, 95, 100, 100, 10, true},
		// Bottom-left corner (outside).
		{2, 98, 100, 100, 10, false},
		// Bottom-right corner (inside).
		{95, 95, 100, 100, 10, true},
		// Bottom-right corner (outside).
		{98, 98, 100, 100, 10, false},
		// Edge (inside).
		{10, 50, 100, 100, 10, true},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			got := insideRoundedRect(tt.x, tt.y, tt.w, tt.h, tt.r)
			if got != tt.want {
				t.Errorf("insideRoundedRect(%d,%d,%d,%d,%d) = %v, want %v",
					tt.x, tt.y, tt.w, tt.h, tt.r, got, tt.want)
			}
		})
	}
}

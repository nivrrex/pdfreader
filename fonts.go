package main

import (
	_ "embed"
	"fmt"

	"gioui.org/font"
	"gioui.org/font/opentype"
	"gioui.org/text"
	"gioui.org/widget/material"
)

// 中文字体随二进制一起发布，避免依赖系统字体。
//
//go:embed assets/NotoSansSC-Regular.ttf
var notoSansSC []byte

const uiTypeface = font.Typeface("NotoSansSC")

// NewTheme 构造一个仅使用内嵌中文字体的 material.Theme。
func NewTheme() (*material.Theme, error) {
	face, err := opentype.Parse(notoSansSC)
	if err != nil {
		return nil, fmt.Errorf("解析内嵌中文字体: %w", err)
	}

	shaper := text.NewShaper(
		text.NoSystemFonts(),
		text.WithCollection([]font.FontFace{{
			Font: font.Font{Typeface: uiTypeface, Weight: font.Normal},
			Face: face,
		}}),
	)

	th := material.NewTheme()
	th.Shaper = shaper
	th.Face = uiTypeface
	return th, nil
}

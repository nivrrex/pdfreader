package main

import (
	"image"
	"image/color"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

type pickerEntry struct {
	name  string
	path  string
	isDir bool
	click widget.Clickable
}

// picker 是“打开”按钮弹出的简易文件选择面板，只在当前目录内浏览 PDF 和子目录。
type picker struct {
	open  bool
	dir   string
	entry []pickerEntry
	err   string

	list     widget.List
	backdrop widget.Clickable
	closeBtn widget.Clickable
	rowWidth int

	pendingDir  string
	pendingFile string
}

func (ui *UI) togglePicker() {
	if ui.picker.open {
		ui.closePicker()
		return
	}
	dir := "."
	if ui.currentPath != "" {
		dir = filepath.Dir(ui.currentPath)
	}
	ui.picker.list.Axis = layout.Vertical
	ui.picker.open = true
	ui.picker.load(dir)
	ui.barVisible = true
	ui.w.Invalidate()
}

func (p *picker) load(dir string) {
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	p.dir = dir
	p.err = ""
	p.pendingDir = ""
	p.pendingFile = ""
	p.entry = p.entry[:0]
	p.list.Position = layout.Position{}

	if parent := filepath.Dir(dir); parent != dir {
		p.entry = append(p.entry, pickerEntry{name: "..", path: parent, isDir: true})
	}

	items, err := os.ReadDir(dir)
	if err != nil {
		p.err = err.Error()
		return
	}

	var dirs, pdfs []pickerEntry
	for _, item := range items {
		name := item.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		full := filepath.Join(dir, name)
		switch {
		case item.IsDir():
			dirs = append(dirs, pickerEntry{name: name + "/", path: full, isDir: true})
		case strings.EqualFold(filepath.Ext(name), ".pdf"):
			pdfs = append(pdfs, pickerEntry{name: name, path: full})
		}
	}
	byName := func(list []pickerEntry) {
		sort.Slice(list, func(i, j int) bool {
			return strings.ToLower(list[i].name) < strings.ToLower(list[j].name)
		})
	}
	byName(dirs)
	byName(pdfs)
	p.entry = append(p.entry, dirs...)
	p.entry = append(p.entry, pdfs...)
}

func (ui *UI) pickerView(gtx layout.Context) {
	layout.Stack{Alignment: layout.Center}.Layout(gtx,
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			if ui.picker.backdrop.Clicked(gtx) {
				ui.closePicker()
			}
			return ui.picker.backdrop.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				paint.Fill(gtx.Ops, color.NRGBA{A: 0x88})
				return layout.Dimensions{Size: gtx.Constraints.Max}
			})
		}),
		layout.Stacked(ui.pickerPanel),
	)
}

func (ui *UI) pickerPanel(gtx layout.Context) layout.Dimensions {
	margin := gtx.Dp(unit.Dp(16))
	w := gtx.Dp(unit.Dp(560))
	// 条目少时收紧面板高度，避免遮挡过多阅读区域；上限 440dp。
	hDP := 90 + 30*len(ui.picker.entry)
	if hDP > 440 {
		hDP = 440
	}
	if hDP < 180 {
		hDP = 180
	}
	h := gtx.Dp(unit.Dp(hDP))
	if max := gtx.Constraints.Max.X - 2*margin; w > max {
		w = max
	}
	if max := gtx.Constraints.Max.Y - 2*margin; h > max {
		h = max
	}
	if w < 240 {
		w = gtx.Constraints.Max.X
	}
	if h < 180 {
		h = gtx.Constraints.Max.Y
	}
	size := image.Pt(w, h)

	defer clip.Rect{Max: size}.Push(gtx.Ops).Pop()
	paint.Fill(gtx.Ops, color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF})
	drawBorder := func(r image.Rectangle) {
		defer clip.Rect(r).Push(gtx.Ops).Pop()
		paint.Fill(gtx.Ops, color.NRGBA{R: 0x99, G: 0x99, B: 0x99, A: 0xFF})
	}
	drawBorder(image.Rect(0, 0, w, 1))
	drawBorder(image.Rect(0, h-1, w, h))
	drawBorder(image.Rect(0, 0, 1, h))
	drawBorder(image.Rect(w-1, 0, w, h))

	gtx.Constraints = layout.Exact(size)
	ui.picker.rowWidth = w - 2*gtx.Dp(unit.Dp(10))
	return layout.UniformInset(unit.Dp(10)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return material.Label(ui.th, unit.Sp(13), "当前目录："+ui.picker.dir).Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				if len(ui.picker.entry) == 0 {
					return layout.Center.Layout(gtx, material.Label(ui.th, unit.Sp(15), "此目录没有 PDF").Layout)
				}
				dims := material.List(ui.th, &ui.picker.list).Layout(gtx, len(ui.picker.entry), func(gtx layout.Context, i int) layout.Dimensions {
					return ui.pickerRow(gtx, i)
				})
				ui.applyPickerPending()
				return dims
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if ui.picker.err == "" {
					return layout.Dimensions{}
				}
				lbl := material.Label(ui.th, unit.Sp(12), ui.picker.err)
				lbl.Color = color.NRGBA{R: 0xB0, G: 0x00, B: 0x00, A: 0xFF}
				return lbl.Layout(gtx)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				for ui.picker.closeBtn.Clicked(gtx) {
					ui.closePicker()
				}
				return layout.Flex{Spacing: layout.SpaceEnd}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return ui.compactButton(gtx, &ui.picker.closeBtn, "取消")
					}),
				)
			}),
		)
	})
}

func (ui *UI) pickerRow(gtx layout.Context, i int) layout.Dimensions {
	e := &ui.picker.entry[i]
	for e.click.Clicked(gtx) {
		if e.isDir {
			ui.picker.pendingDir = e.path
		} else {
			ui.picker.pendingFile = e.path
		}
	}
	dims := e.click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		d := layout.UniformInset(unit.Dp(5)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			lbl := material.Label(ui.th, unit.Sp(15), e.name)
			if e.isDir {
				lbl.Color = ui.th.Palette.ContrastBg
			}
			return lbl.Layout(gtx)
		})
		return layout.Dimensions{Size: image.Pt(ui.picker.rowWidth, d.Size.Y)}
	})
	return dims
}

// closePicker 关闭面板并请求重绘，避免点击帧仍绘制面板、之后没有新帧。
func (ui *UI) closePicker() {
	if ui.picker.open {
		ui.picker.open = false
		ui.w.Invalidate()
	}
}

// applyPickerPending 在列表布局结束后再执行目录切换或打开文件，避免布局中途替换条目。
func (ui *UI) applyPickerPending() {
	if dir := ui.picker.pendingDir; dir != "" {
		ui.picker.pendingDir = ""
		ui.picker.load(dir)
		ui.w.Invalidate()
		return
	}
	file := ui.picker.pendingFile
	if file == "" {
		return
	}
	ui.picker.pendingFile = ""
	if err := ui.Open(file); err != nil {
		ui.picker.err = err.Error()
		return
	}
	ui.picker.open = false
}

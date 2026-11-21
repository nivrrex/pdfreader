package main

import (
	"image"
	"image/color"
	"log"
	"math"
	"os"
	"time"

	"gioui.org/app"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

// UI 持有视图状态；所有字段只在事件循环 goroutine 中访问。
type UI struct {
	w  *app.Window
	th *material.Theme
	r  *Reader

	fullscreen  bool
	status      string
	statusError bool

	windowedPx image.Point // 最近一次非全屏窗口的像素尺寸
	fullPx     image.Point // 全屏时的像素尺寸
	pxPerDp    float32
	restoring  bool // 刚退出全屏，等待窗口尺寸恢复
	lastScroll time.Time

	barVisible bool // 顶部按钮栏是否显示
	barTag     int

	currentPath string // 当前打开的 PDF 路径
	picker      picker

	btnOpen       widget.Clickable
	btnPrev       widget.Clickable
	btnNext       widget.Clickable
	btnZoomIn     widget.Clickable
	btnZoomOut    widget.Clickable
	btnActual     widget.Clickable
	btnFitPage    widget.Clickable
	btnFitWidth   widget.Clickable
	btnFullscreen widget.Clickable

	fit      fitMode
	pan      image.Point
	topAlign bool // 下一次绘制时把页面顶端对齐视口
	dragging bool
	dragLast image.Point

	scrollTag int
	dragTag   int
}

type fitMode uint8

const (
	fitNone fitMode = iota
	fitPage
	fitWidth
)

func NewUI(w *app.Window, th *material.Theme) *UI {
	return &UI{w: w, th: th, barVisible: true}
}

func (ui *UI) Layout(gtx layout.Context) layout.Dimensions {
	ui.trackWindowSize(gtx)
	ui.updateBar(gtx)
	ui.handleKeys(gtx)
	ui.applyFit(gtx)

	dims := layout.Stack{Alignment: layout.NW}.Layout(gtx,
		layout.Stacked(ui.page),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			if !ui.barVisible {
				return layout.Dimensions{}
			}
			return ui.toolbar(gtx)
		}),
	)
	if ui.picker.open {
		ui.pickerView(gtx)
	}
	return dims
}

func (ui *UI) toolbar(gtx layout.Context) layout.Dimensions {
	items := []struct {
		click  *widget.Clickable
		label  string
		action func()
		group  int
	}{
		{&ui.btnOpen, "打开", ui.togglePicker, 0},
		{&ui.btnPrev, "上一页", ui.prevPage, 1},
		{&ui.btnNext, "下一页", ui.nextPage, 1},
		{&ui.btnZoomIn, "放大", ui.zoomIn, 2},
		{&ui.btnZoomOut, "缩小", ui.zoomOut, 2},
		{&ui.btnActual, "实际大小", ui.actualSize, 3},
		{&ui.btnFitPage, "适合页面", ui.fitPageView, 3},
		{&ui.btnFitWidth, "适合宽度", ui.fitWidthView, 3},
		{&ui.btnFullscreen, "全屏", ui.toggleFullscreen, 4},
	}

	children := make([]layout.FlexChild, 0, len(items))
	for i := range items {
		it := items[i]
		// 同组按钮间距一致，不同功能组之间加大间隔。
		gap := unit.Dp(6)
		if i == len(items)-1 {
			gap = 0
		} else if items[i+1].group != it.group {
			gap = unit.Dp(24)
		}
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			// Gio v0.10 中 Clicked 必须在 Layout 之前调用：Layout 内部会消费并丢弃点击事件。
			for it.click.Clicked(gtx) {
				it.action()
			}
			return layout.Inset{Right: gap}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return ui.compactButton(gtx, it.click, it.label)
			})
		}))
	}

	// 先录制按钮内容，再画背景，保证背景不遮挡按钮事件与文字。
	macro := op.Record(gtx.Ops)
	dims := layout.UniformInset(unit.Dp(3)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
	})
	content := macro.Stop()

	bar := clip.Rect{Max: image.Pt(gtx.Constraints.Max.X, dims.Size.Y)}.Push(gtx.Ops)
	paint.Fill(gtx.Ops, ui.th.Palette.Bg)
	divider := clip.Rect{Min: image.Pt(0, dims.Size.Y-1), Max: image.Pt(gtx.Constraints.Max.X, dims.Size.Y)}.Push(gtx.Ops)
	paint.Fill(gtx.Ops, color.NRGBA{R: 0xD8, G: 0xD8, B: 0xD8, A: 0xFF})
	divider.Pop()
	bar.Pop()
	content.Add(gtx.Ops)
	return dims
}

func (ui *UI) compactButton(gtx layout.Context, b *widget.Clickable, label string) layout.Dimensions {
	btn := material.Button(ui.th, b, label)
	btn.TextSize = unit.Sp(12)
	btn.Inset = layout.Inset{Top: unit.Dp(3), Bottom: unit.Dp(3), Left: unit.Dp(8), Right: unit.Dp(8)}
	btn.CornerRadius = 3
	return btn.Layout(gtx)
}

// updateBar 根据鼠标纵向位置自动显示/隐藏顶部按钮栏。
func (ui *UI) updateBar(gtx layout.Context) {
	event.Op(gtx.Ops, &ui.barTag)
	for {
		ev, ok := gtx.Event(pointer.Filter{Target: &ui.barTag, Kinds: pointer.Move | pointer.Enter})
		if !ok {
			break
		}
		pe, ok := ev.(pointer.Event)
		if !ok || gtx.Metric.PxPerDp <= 0 {
			continue
		}
		y := pe.Position.Y / gtx.Metric.PxPerDp
		switch {
		case y <= 64:
			ui.barVisible = true
		case y >= 96:
			ui.barVisible = false
		}
	}
	if ui.picker.open {
		ui.barVisible = true
	}
}

// page 绘制页面区域，同时监听该区域内的滚轮事件。
func (ui *UI) page(gtx layout.Context) layout.Dimensions {
	// 全屏切换时约束会突变，先裁剪，避免绘制溢出到工具栏或窗口外。
	defer clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops).Pop()
	// 浅灰底色让白底页面的实际边界可见（尤其是“实际大小”时）。
	paint.Fill(gtx.Ops, color.NRGBA{R: 0xE8, G: 0xE8, B: 0xE8, A: 0xFF})

	// 注册滚轮事件目标，范围覆盖整个页面区域。
	event.Op(gtx.Ops, &ui.scrollTag)
	ui.handleScroll(gtx)
	ui.handlePageDrag(gtx)

	if ui.r == nil || ui.r.pageImage == nil {
		msg := ui.status
		if msg == "" {
			msg = "请打开 PDF"
		}
		return ui.center(gtx, material.Label(ui.th, unit.Sp(18), msg).Layout)
	}

	// 页面位图按物理像素 1:1 绘制；缩放通过重新渲染的 DPI 体现。
	// 这里不使用 widget.Image 的 Fit，因为它会把图像裁到窗口大小，
	// 拖动时只会移动那一块，无法露出页面其余部分。
	size := ui.r.pageSize
	if ui.topAlign {
		// 交给 clampPan 夹到最大纵向偏移，即页面顶端对齐窗口顶端。
		ui.pan.Y = math.MaxInt
		ui.topAlign = false
	}
	ui.clampPan(gtx.Constraints.Max, size)
	off := image.Pt(
		(gtx.Constraints.Max.X-size.X)/2+ui.pan.X,
		(gtx.Constraints.Max.Y-size.Y)/2+ui.pan.Y,
	)
	trans := op.Offset(off).Push(gtx.Ops)
	imgClip := clip.Rect{Max: size}.Push(gtx.Ops)
	paint.NewImageOp(ui.r.pageImage).Add(gtx.Ops)
	paint.PaintOp{}.Add(gtx.Ops)
	imgClip.Pop()
	trans.Pop()
	ui.drawStatusError(gtx)
	return layout.Dimensions{Size: gtx.Constraints.Max}
}

func (ui *UI) drawStatusError(gtx layout.Context) {
	if !ui.statusError || ui.status == "" {
		return
	}
	macro := op.Record(gtx.Ops)
	dims := layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
		label := material.Label(ui.th, unit.Sp(13), ui.status)
		label.Color = color.NRGBA{R: 0x8B, G: 0x1A, B: 0x1A, A: 0xFF}
		return label.Layout(gtx)
	})
	content := macro.Stop()
	y := max(0, gtx.Constraints.Max.Y-dims.Size.Y)
	bar := clip.Rect{Min: image.Pt(0, y), Max: gtx.Constraints.Max}.Push(gtx.Ops)
	paint.Fill(gtx.Ops, color.NRGBA{R: 0xFF, G: 0xF2, B: 0xF2, A: 0xF2})
	bar.Pop()
	trans := op.Offset(image.Pt(0, y)).Push(gtx.Ops)
	content.Add(gtx.Ops)
	trans.Pop()
}

// handlePageDrag 支持按住左键拖动页面，用于放大后查看被窗口裁掉的区域。
func (ui *UI) handlePageDrag(gtx layout.Context) {
	if ui.r == nil || ui.picker.open {
		return
	}
	if ui.barVisible {
		// 顶部按钮栏可见时不让页面拖动抢占按钮的点击。
		defer clip.Rect{Min: image.Pt(0, gtx.Dp(unit.Dp(56))), Max: gtx.Constraints.Max}.Push(gtx.Ops).Pop()
	}
	event.Op(gtx.Ops, &ui.dragTag)
	for {
		ev, ok := gtx.Event(pointer.Filter{
			Target: &ui.dragTag,
			Kinds:  pointer.Press | pointer.Drag | pointer.Release | pointer.Cancel,
		})
		if !ok {
			break
		}
		pe, ok := ev.(pointer.Event)
		if !ok {
			continue
		}
		pos := pe.Position.Round()
		switch pe.Kind {
		case pointer.Press:
			if pe.Buttons == pointer.ButtonPrimary || pe.Source == pointer.Touch {
				ui.dragging = true
				ui.dragLast = pos
			}
		case pointer.Drag:
			if ui.dragging {
				ui.pan = ui.pan.Add(pos.Sub(ui.dragLast))
				ui.dragLast = pos
				ui.clampPan(gtx.Constraints.Max, ui.r.pageSize)
			}
		case pointer.Release, pointer.Cancel:
			ui.dragging = false
		}
	}
}

func (ui *UI) clampPan(view, page image.Point) {
	maxX := (page.X - view.X) / 2
	if maxX < 0 {
		maxX = 0
	}
	maxY := (page.Y - view.Y) / 2
	if maxY < 0 {
		maxY = 0
	}
	if ui.pan.X > maxX {
		ui.pan.X = maxX
	}
	if ui.pan.X < -maxX {
		ui.pan.X = -maxX
	}
	if ui.pan.Y > maxY {
		ui.pan.Y = maxY
	}
	if ui.pan.Y < -maxY {
		ui.pan.Y = -maxY
	}
}

// center 在可用区域内居中一个内容自适应尺寸的控件。
// Stack 会把 Constraints.Min 清零，直接 layout.Center 会退化为左上对齐。
func (ui *UI) center(gtx layout.Context, w layout.Widget) layout.Dimensions {
	gtx.Constraints.Min = gtx.Constraints.Max
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min = image.Point{}
		return w(gtx)
	})
}

func (ui *UI) handleKeys(gtx layout.Context) {
	for {
		ev, ok := gtx.Event(key.Filter{})
		if !ok {
			return
		}
		ke, ok := ev.(key.Event)
		if !ok || ke.State != key.Press {
			continue
		}
		if ui.picker.open {
			if ke.Name == key.NameEscape {
				ui.closePicker()
			}
			continue
		}
		switch ke.Name {
		case "+", "=":
			ui.zoomIn()
		case "-":
			ui.zoomOut()
		case key.NamePageUp, key.NameLeftArrow, key.NameUpArrow:
			ui.prevPage()
		case key.NamePageDown, key.NameRightArrow, key.NameDownArrow:
			ui.nextPage()
		case key.NameF11:
			ui.toggleFullscreen()
		case key.NameEscape:
			ui.setFullscreen(false)
		}
	}
}

func (ui *UI) handleScroll(gtx layout.Context) {
	var dy float32
	var zoom bool
	for {
		ev, ok := gtx.Event(pointer.Filter{
			Target:  &ui.scrollTag,
			Kinds:   pointer.Scroll,
			ScrollY: pointer.ScrollRange{Min: -1 << 20, Max: 1 << 20},
		})
		if !ok {
			break
		}
		pe, ok := ev.(pointer.Event)
		if !ok || pe.Kind != pointer.Scroll {
			continue
		}
		dy += pe.Scroll.Y
		if pe.Modifiers.Contain(key.ModCtrl) {
			zoom = true
		}
	}
	if dy == 0 {
		return
	}
	// X11 滚轮按下和释放都会产生 Scroll 事件，40ms 内只响应一次，
	// 保证一格滚轮只翻一页或只缩放一级。
	if !ui.lastScroll.IsZero() && gtx.Now.Sub(ui.lastScroll) < 40*time.Millisecond {
		return
	}
	ui.lastScroll = gtx.Now

	switch {
	case zoom && dy < 0:
		ui.zoomIn()
	case zoom && dy > 0:
		ui.zoomOut()
	case dy < 0:
		ui.prevPage()
	case dy > 0:
		ui.nextPage()
	}
}

// openStartup 只打开命令行传入的文件；没有参数时等待用户点“打开”选择。
func (ui *UI) openStartup() {
	if len(os.Args) < 2 {
		return
	}
	path := os.Args[1]
	if err := ui.Open(path); err != nil {
		ui.status = "无法打开 " + path + "：" + err.Error()
		ui.statusError = true
		log.Printf("打开 PDF 失败: %v", err)
	}
}

// Open 打开新文档；成功后才替换旧文档。
func (ui *UI) Open(path string) error {
	r, err := OpenReader(path)
	if err != nil {
		return err
	}
	if ui.r != nil {
		_ = ui.r.Close()
	}
	ui.r = r
	ui.currentPath = path
	ui.status = ""
	ui.statusError = false
	ui.fit = fitNone
	ui.pan = image.Point{}
	ui.topAlign = false
	ui.w.Invalidate()
	return nil
}

func (ui *UI) Close() {
	if ui.r != nil {
		_ = ui.r.Close()
		ui.r = nil
	}
}

func (ui *UI) prevPage() {
	if ui.r == nil {
		return
	}
	before := ui.r.PageIndex()
	if err := ui.r.PrevPage(); err != nil {
		ui.apply(err)
		return
	}
	if ui.r.PageIndex() == before {
		return
	}
	ui.pan = image.Point{}
	ui.topAlign = true
	ui.apply(nil)
}

func (ui *UI) nextPage() {
	if ui.r == nil {
		return
	}
	before := ui.r.PageIndex()
	if err := ui.r.NextPage(); err != nil {
		ui.apply(err)
		return
	}
	if ui.r.PageIndex() == before {
		return
	}
	ui.pan = image.Point{}
	ui.topAlign = true
	ui.apply(nil)
}

func (ui *UI) zoomIn() {
	if ui.r == nil {
		return
	}
	before := ui.r.Zoom()
	if err := ui.r.ZoomIn(); err != nil {
		ui.apply(err)
		return
	}
	if ui.r.Zoom() == before {
		return
	}
	ui.fit = fitNone
	ui.pan = image.Point{}
	ui.topAlign = false
	ui.apply(nil)
}

func (ui *UI) zoomOut() {
	if ui.r == nil {
		return
	}
	before := ui.r.Zoom()
	if err := ui.r.ZoomOut(); err != nil {
		ui.apply(err)
		return
	}
	if ui.r.Zoom() == before {
		return
	}
	ui.fit = fitNone
	ui.pan = image.Point{}
	ui.topAlign = false
	ui.apply(nil)
}

// actualSize 按 100% 显示：一个 PDF 点对应一个逻辑像素（dp）。
func (ui *UI) actualSize() {
	if ui.r == nil {
		return
	}
	scale := float64(ui.pxPerDp)
	if scale <= 0 {
		scale = 1
	}
	before := ui.r.Zoom()
	if err := ui.r.SetZoom(scale); err != nil {
		ui.apply(err)
		return
	}
	if ui.r.Zoom() == before {
		return
	}
	ui.fit = fitNone
	ui.pan = image.Point{}
	ui.topAlign = true
	ui.apply(nil)
}

// fitPageView 让整页适应窗口；窗口尺寸变化时会持续重算。
func (ui *UI) fitPageView() {
	if ui.r == nil {
		return
	}
	ui.fit = fitPage
	ui.pan = image.Point{}
	ui.topAlign = false
	ui.w.Invalidate()
}

// fitWidthView 让页面宽度适应窗口；高度超出部分可拖动查看。
func (ui *UI) fitWidthView() {
	if ui.r == nil {
		return
	}
	ui.fit = fitWidth
	ui.pan = image.Point{}
	ui.topAlign = true
	ui.w.Invalidate()
}

// applyFit 在布局阶段按当前窗口尺寸计算并应用适配缩放。
func (ui *UI) applyFit(gtx layout.Context) {
	if ui.r == nil || ui.fit == fitNone {
		return
	}
	base := ui.r.BaseSize()
	view := gtx.Constraints.Max
	if base.X <= 0 || base.Y <= 0 || view.X <= 0 || view.Y <= 0 {
		return
	}
	var zoom float64
	switch ui.fit {
	case fitPage:
		zoom = math.Min(float64(view.X)/float64(base.X), float64(view.Y)/float64(base.Y))
	case fitWidth:
		zoom = float64(view.X) / float64(base.X)
	default:
		return
	}
	zoom = math.Max(minZoom, math.Min(maxZoom, zoom))
	if math.Abs(zoom-ui.r.Zoom()) < 0.001 {
		return
	}
	if err := ui.r.SetZoom(zoom); err != nil {
		ui.fit = fitNone
		ui.apply(err)
		return
	}
	ui.pan = image.Point{}
	ui.topAlign = ui.fit == fitWidth
	ui.apply(nil)
}

// apply 记录错误并请求重绘；页面状态变更后必须重绘。
func (ui *UI) apply(err error) {
	if err != nil {
		ui.status = err.Error()
		ui.statusError = true
		log.Printf("PDF 操作失败: %v", err)
	} else {
		ui.status = ""
		ui.statusError = false
	}
	ui.w.Invalidate()
}

func (ui *UI) toggleFullscreen() {
	ui.setFullscreen(!ui.fullscreen)
}

// trackWindowSize 记录非全屏尺寸，供退出全屏时恢复。
func (ui *UI) trackWindowSize(gtx layout.Context) {
	size := gtx.Constraints.Max
	switch {
	case ui.fullscreen:
		ui.fullPx = size
	case ui.restoring:
		if size != ui.fullPx {
			ui.restoring = false
			ui.windowedPx = size
			ui.pxPerDp = gtx.Metric.PxPerDp
		}
	default:
		ui.windowedPx = size
		ui.pxPerDp = gtx.Metric.PxPerDp
	}
}

// setFullscreen 通过窗口模式选项切换全屏（Gio 没有 SetFullscreen 方法）。
func (ui *UI) setFullscreen(on bool) {
	if ui.fullscreen == on {
		return
	}
	ui.fullscreen = on
	if on {
		ui.w.Option(app.WindowMode(app.Fullscreen).Option())
	} else {
		// 退出全屏时显式恢复进入前的窗口尺寸：X11 后端不会自动还原。
		opts := []app.Option{app.WindowMode(app.Windowed).Option()}
		if ui.windowedPx.X > 0 && ui.windowedPx.Y > 0 {
			scale := ui.pxPerDp
			if scale <= 0 {
				scale = 1
			}
			ui.restoring = true
			opts = append(opts, app.Size(
				unit.Dp(float32(ui.windowedPx.X)/scale),
				unit.Dp(float32(ui.windowedPx.Y)/scale),
			))
		}
		ui.w.Option(opts...)
	}
	ui.w.Invalidate()
}

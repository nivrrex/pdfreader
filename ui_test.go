package main

import (
	"image"
	"testing"
)

func TestBoundaryActionsPreserveView(t *testing.T) {
	r := openTestReader(t)
	ui := &UI{
		r:           r,
		fit:         fitWidth,
		pan:         image.Pt(7, 9),
		status:      "旧错误",
		statusError: true,
	}
	wantPan := ui.pan

	ui.prevPage()
	if ui.pan != wantPan || ui.fit != fitWidth || !ui.statusError {
		t.Fatal("首页上一页改变了视图或错误状态")
	}

	ui.pxPerDp = 1
	ui.actualSize()
	if ui.pan != wantPan || ui.fit != fitWidth || !ui.statusError {
		t.Fatal("实际大小无变化时改变了视图或错误状态")
	}

	if err := r.SetZoom(minZoom); err != nil {
		t.Fatal(err)
	}
	ui.zoomOut()
	if ui.pan != wantPan || ui.fit != fitWidth || !ui.statusError {
		t.Fatal("缩到下限改变了视图或错误状态")
	}

	if err := r.SetZoom(maxZoom); err != nil {
		t.Fatal(err)
	}
	ui.zoomIn()
	if ui.pan != wantPan || ui.fit != fitWidth || !ui.statusError {
		t.Fatal("放到上限改变了视图或错误状态")
	}
}

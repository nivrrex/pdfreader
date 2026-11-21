package main

import (
	"errors"
	"fmt"
	"image"
	"math"
	"os"
	"path/filepath"

	"github.com/qiangmzsx/go-fitz"
)

const (
	minZoom = 0.2
	maxZoom = 5.0

	// baseDPI 对应 zoom == 1.0；MuPDF 以 72 DPI 表示页面原始尺寸。
	baseDPI = 72.0

	// 渲染 DPI 的实用上下限，避免极小 DPI 模糊与极大 DPI 内存爆炸。
	minDPI = 24.0
	maxDPI = 600.0

	zoomStep = 1.2
)

// Reader 封装 go-fitz 文档状态。
// 非线程安全：所有方法必须在同一个 goroutine 中调用，ImageDPI 也因而保持顺序执行。
type Reader struct {
	doc       *fitz.Document
	pageIndex int
	zoom      float64
	pageImage image.Image
	pageSize  image.Point
	baseSize  image.Point // 当前页在 72 DPI（PDF 点）下的尺寸
}

type renderedPage struct {
	image    image.Image
	size     image.Point
	baseSize image.Point
}

// OpenReader 打开文档并渲染首页。
func OpenReader(path string) (*Reader, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("解析 PDF 路径 %q: %w", path, err)
	}
	if _, err := os.Stat(absPath); err != nil {
		return nil, fmt.Errorf("读取 PDF %q: %w", absPath, err)
	}

	doc, err := fitz.New(absPath)
	if err != nil {
		// go-fitz 在打开失败时可能已分配 context；这两个哨兵表示 context 尚未创建。
		if doc != nil && !errors.Is(err, fitz.ErrNoSuchFile) && !errors.Is(err, fitz.ErrCreateContext) {
			_ = doc.Close() // go-fitz Close 当前不会返回错误。
		}
		return nil, fmt.Errorf("打开 PDF %q: %w", absPath, err)
	}
	r := &Reader{doc: doc, zoom: 1.0}
	if err := r.Render(); err != nil {
		_ = doc.Close() // go-fitz Close 当前不会返回错误。
		return nil, err
	}
	return r, nil
}

func (r *Reader) renderPage(pageIndex int, zoom float64) (renderedPage, error) {
	if r.doc == nil {
		return renderedPage{}, errors.New("reader: 文档未打开")
	}
	dpi := baseDPI * zoom
	if dpi < minDPI {
		dpi = minDPI
	}
	if dpi > maxDPI {
		dpi = maxDPI
	}
	img, err := r.doc.ImageDPI(pageIndex, dpi)
	if err != nil {
		return renderedPage{}, fmt.Errorf("渲染第 %d 页（%.0f DPI）: %w", pageIndex+1, dpi, err)
	}
	bound, err := r.doc.Bound(pageIndex)
	if err != nil {
		return renderedPage{}, fmt.Errorf("读取第 %d 页尺寸: %w", pageIndex+1, err)
	}
	return renderedPage{image: img, size: img.Bounds().Size(), baseSize: bound.Size()}, nil
}

// renderAndCommit 在渲染和尺寸读取均成功后提交状态，避免失败后页码、缩放与旧位图不一致。
func (r *Reader) renderAndCommit(pageIndex int, zoom float64) error {
	page, err := r.renderPage(pageIndex, zoom)
	if err != nil {
		return err
	}
	r.pageIndex = pageIndex
	r.zoom = zoom
	r.pageImage = page.image
	r.pageSize = page.size
	r.baseSize = page.baseSize
	return nil
}

// Render 按当前 zoom 换算 DPI 并重新渲染当前页。
func (r *Reader) Render() error {
	return r.renderAndCommit(r.pageIndex, r.zoom)
}

// SetZoom 设置缩放并重新渲染；zoom 会钳制到 [0.2, 5.0]。
func (r *Reader) SetZoom(zoom float64) error {
	if r.doc == nil {
		return nil
	}
	zoom = math.Max(minZoom, math.Min(maxZoom, zoom))
	if zoom == r.zoom {
		return nil
	}
	return r.renderAndCommit(r.pageIndex, zoom)
}

// Zoom 返回当前缩放值。
func (r *Reader) Zoom() float64 {
	return r.zoom
}

// BaseSize 返回当前页在 72 DPI 下的尺寸（PDF 点）。
func (r *Reader) BaseSize() image.Point {
	return r.baseSize
}

// PrevPage 翻到上一页；已在首页时不做任何事。
func (r *Reader) PrevPage() error {
	if r.doc == nil || r.pageIndex <= 0 {
		return nil
	}
	return r.renderAndCommit(r.pageIndex-1, r.zoom)
}

// NextPage 翻到下一页；已在末页时不做任何事。
func (r *Reader) NextPage() error {
	if r.doc == nil || r.pageIndex >= r.doc.NumPage()-1 {
		return nil
	}
	return r.renderAndCommit(r.pageIndex+1, r.zoom)
}

// ZoomIn 放大一级，上限 5.0。
func (r *Reader) ZoomIn() error {
	if r.doc == nil {
		return nil
	}
	return r.SetZoom(r.zoom * zoomStep)
}

// ZoomOut 缩小一级，下限 0.2。
func (r *Reader) ZoomOut() error {
	if r.doc == nil {
		return nil
	}
	return r.SetZoom(r.zoom / zoomStep)
}

// PageIndex 返回当前页的零基索引；文档未打开时为 0。
func (r *Reader) PageIndex() int {
	return r.pageIndex
}

// PageCount 返回总页数；文档未打开时为 0。
func (r *Reader) PageCount() int {
	if r.doc == nil {
		return 0
	}
	return r.doc.NumPage()
}

// Close 释放文档；可重复调用。
func (r *Reader) Close() error {
	if r.doc == nil {
		return nil
	}
	err := r.doc.Close()
	r.doc = nil
	r.pageImage = nil
	r.pageSize = image.Point{}
	return err
}

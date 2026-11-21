package main

import (
	"errors"
	"image"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/qiangmzsx/go-fitz"
)

type readerState struct {
	pageIndex int
	zoom      float64
	pageImage image.Image
	pageSize  image.Point
	baseSize  image.Point
}

func stateOf(r *Reader) readerState {
	return readerState{
		pageIndex: r.PageIndex(),
		zoom:      r.Zoom(),
		pageImage: r.pageImage,
		pageSize:  r.pageSize,
		baseSize:  r.baseSize,
	}
}

func assertReaderState(t *testing.T, got, want readerState) {
	t.Helper()
	if got.pageIndex != want.pageIndex || got.zoom != want.zoom ||
		got.pageImage != want.pageImage || got.pageSize != want.pageSize ||
		got.baseSize != want.baseSize {
		t.Fatalf("Reader 状态发生变化：got %+v，want %+v", got, want)
	}
}

func openTestReader(t *testing.T) *Reader {
	t.Helper()
	r, err := OpenReader(filepath.Join("testdata", "test-cn.pdf"))
	if err != nil {
		t.Fatalf("OpenReader() 失败：%v", err)
	}
	t.Cleanup(func() {
		if err := r.Close(); err != nil {
			t.Errorf("Close() 失败：%v", err)
		}
	})
	return r
}

func TestOpenReaderAndPageBoundaries(t *testing.T) {
	r := openTestReader(t)
	if got := r.PageCount(); got != 3 {
		t.Fatalf("PageCount() = %d，want 3", got)
	}
	if got := r.BaseSize(); got != image.Pt(595, 841) {
		t.Fatalf("BaseSize() = %v，want (595,841)", got)
	}
	if r.pageImage == nil || r.pageSize == (image.Point{}) {
		t.Fatal("首页未渲染")
	}

	before := stateOf(r)
	if err := r.PrevPage(); err != nil {
		t.Fatalf("首页 PrevPage() 失败：%v", err)
	}
	assertReaderState(t, stateOf(r), before)

	for want := 1; want < r.PageCount(); want++ {
		if err := r.NextPage(); err != nil {
			t.Fatalf("NextPage() 到第 %d 页失败：%v", want+1, err)
		}
		if got := r.PageIndex(); got != want {
			t.Fatalf("PageIndex() = %d，want %d", got, want)
		}
	}

	before = stateOf(r)
	if err := r.NextPage(); err != nil {
		t.Fatalf("末页 NextPage() 失败：%v", err)
	}
	assertReaderState(t, stateOf(r), before)

	if err := r.PrevPage(); err != nil {
		t.Fatalf("PrevPage() 失败：%v", err)
	}
	if got := r.PageIndex(); got != 1 {
		t.Fatalf("PageIndex() = %d，want 1", got)
	}
}

func TestSetZoomClampsAndBoundaryNoOps(t *testing.T) {
	r := openTestReader(t)
	if err := r.SetZoom(-1); err != nil {
		t.Fatalf("SetZoom(-1) 失败：%v", err)
	}
	if got := r.Zoom(); got != minZoom {
		t.Fatalf("Zoom() = %v，want %v", got, minZoom)
	}
	if r.pageSize.X >= r.baseSize.X {
		t.Fatalf("缩小后的页面宽度 %d 不小于基准宽度 %d", r.pageSize.X, r.baseSize.X)
	}

	before := stateOf(r)
	if err := r.ZoomOut(); err != nil {
		t.Fatalf("下限 ZoomOut() 失败：%v", err)
	}
	assertReaderState(t, stateOf(r), before)

	if err := r.SetZoom(maxZoom + 1); err != nil {
		t.Fatalf("SetZoom(maxZoom+1) 失败：%v", err)
	}
	if got := r.Zoom(); got != maxZoom {
		t.Fatalf("Zoom() = %v，want %v", got, maxZoom)
	}

	before = stateOf(r)
	if err := r.ZoomIn(); err != nil {
		t.Fatalf("上限 ZoomIn() 失败：%v", err)
	}
	assertReaderState(t, stateOf(r), before)
}

func TestRenderFailureRollsBackState(t *testing.T) {
	r := openTestReader(t)
	if err := r.NextPage(); err != nil {
		t.Fatalf("NextPage() 失败：%v", err)
	}
	before := stateOf(r)
	missingPage := r.PageCount()
	err := r.renderAndCommit(missingPage, r.Zoom()*zoomStep)
	if !errors.Is(err, fitz.ErrPageMissing) {
		t.Fatalf("renderAndCommit() 错误 = %v，want ErrPageMissing", err)
	}
	if !strings.Contains(err.Error(), strconv.Itoa(missingPage+1)) ||
		!strings.Contains(err.Error(), "DPI") {
		t.Fatalf("错误缺少页码或 DPI 上下文：%v", err)
	}
	assertReaderState(t, stateOf(r), before)
}

func TestOpenReaderErrors(t *testing.T) {
	t.Run("missing", func(t *testing.T) {
		r, err := OpenReader(filepath.Join(t.TempDir(), "missing.pdf"))
		if r != nil {
			_ = r.Close()
			t.Fatal("文件不存在时返回了 Reader")
		}
		if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("错误 = %v，want os.ErrNotExist", err)
		}
	})

	t.Run("invalid", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "invalid.pdf")
		if err := os.WriteFile(path, []byte("not a PDF"), 0o600); err != nil {
			t.Fatal(err)
		}
		r, err := OpenReader(path)
		if r != nil {
			_ = r.Close()
			t.Fatal("无效 PDF 返回了 Reader")
		}
		if !errors.Is(err, fitz.ErrOpenDocument) {
			t.Fatalf("错误 = %v，want ErrOpenDocument", err)
		}
	})

	t.Run("encrypted", func(t *testing.T) {
		r, err := OpenReader(filepath.Join("testdata", "encrypted.pdf"))
		if r != nil {
			_ = r.Close()
			t.Fatal("加密 PDF 返回了 Reader")
		}
		if !errors.Is(err, fitz.ErrNeedsPassword) {
			t.Fatalf("错误 = %v，want ErrNeedsPassword", err)
		}
	})
}

func TestCloseIsIdempotent(t *testing.T) {
	r := openTestReader(t)
	if err := r.Close(); err != nil {
		t.Fatalf("第一次 Close() 失败：%v", err)
	}
	before := stateOf(r)
	if err := r.Close(); err != nil {
		t.Fatalf("第二次 Close() 失败：%v", err)
	}
	assertReaderState(t, stateOf(r), before)
	if got := r.PageCount(); got != 0 {
		t.Fatalf("关闭后 PageCount() = %d，want 0", got)
	}
	if err := r.Render(); err == nil {
		t.Fatal("关闭后 Render() 应返回错误")
	}
	assertReaderState(t, stateOf(r), before)
	if err := r.PrevPage(); err != nil {
		t.Fatalf("关闭后 PrevPage() 失败：%v", err)
	}
	if err := r.NextPage(); err != nil {
		t.Fatalf("关闭后 NextPage() 失败：%v", err)
	}
	if err := r.SetZoom(2); err != nil {
		t.Fatalf("关闭后 SetZoom() 失败：%v", err)
	}
	assertReaderState(t, stateOf(r), before)
}

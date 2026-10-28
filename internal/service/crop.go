package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	"image/draw"
	_ "image/jpeg"
	"image/png"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"onlineTest/internal/ai"
	"onlineTest/internal/domain"
)

func (s *Service) attachQuestionImages(ctx context.Context, questions []ai.ParsedQuestion) {
	multiple := len(questions) > 1
	cache := make(map[string]*domain.File)
	for i := range questions {
		q := &questions[i]
		if strings.TrimSpace(q.SourceFile) == "" {
			continue
		}
		source, ok := cache[q.SourceFile]
		if !ok {
			var err error
			source, err = s.Store.GetFile(ctx, q.SourceFile)
			if err != nil {
				slog.Warn("question source image missing", "file_id", q.SourceFile, "error", err)
				if multiple {
					q.SourceFile = ""
				}
				continue
			}
			cache[q.SourceFile] = source
		}

		box := q.Figure
		if box == nil {
			if multiple {
				q.SourceFile = ""
				continue
			}
			box = q.BBox
		}
		if box == nil {
			continue
		}

		cropped, err := s.cropQuestionImage(ctx, source, *box)
		if err != nil {
			slog.Warn("crop question image failed", "file_id", source.ID, "question_seq", q.Seq, "error", err)
			if multiple {
				q.SourceFile = ""
			}
			continue
		}
		q.SourceFile = cropped.ID
	}
}

func (s *Service) cropQuestionImage(ctx context.Context, source *domain.File, box ai.ImageBox) (*domain.File, error) {
	data, err := os.ReadFile(s.FilePath(source))
	if err != nil {
		return nil, fmt.Errorf("读取源图失败: %w", err)
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("解码源图失败: %w", err)
	}
	rect := imageCropRect(img.Bounds(), box)
	if rect.Empty() {
		return nil, fmt.Errorf("裁剪区域过小")
	}
	rect = growCropRect(img, rect)
	if rect.Dx() < 8 || rect.Dy() < 8 {
		return nil, fmt.Errorf("裁剪区域过小")
	}

	out := image.NewRGBA(image.Rect(0, 0, rect.Dx(), rect.Dy()))
	draw.Draw(out, out.Bounds(), img, rect.Min, draw.Src)

	var buf bytes.Buffer
	if err := png.Encode(&buf, out); err != nil {
		return nil, fmt.Errorf("编码配图失败: %w", err)
	}
	now := time.Now().UTC()
	rel := filepath.Join("uploads", now.Format("2006"), now.Format("01"), uuid.NewString()+".png")
	abs := filepath.Join(s.DataDir, rel)
	if err := os.MkdirAll(filepath.Dir(abs), 0o700); err != nil {
		return nil, err
	}
	if err := os.WriteFile(abs, buf.Bytes(), 0o600); err != nil {
		return nil, err
	}

	sum := sha256.Sum256(buf.Bytes())
	f := &domain.File{
		ID:           uuid.NewString(),
		OwnerID:      source.OwnerID,
		OriginalName: cropName(source.OriginalName),
		StoredPath:   filepath.ToSlash(rel),
		Mime:         "image/png",
		SizeBytes:    int64(buf.Len()),
		SHA256:       hex.EncodeToString(sum[:]),
		Kind:         "question_image",
		Width:        out.Bounds().Dx(),
		Height:       out.Bounds().Dy(),
	}
	if err := s.Store.CreateFile(ctx, f); err != nil {
		_ = os.Remove(abs)
		return nil, err
	}
	return f, nil
}

func cropName(name string) string {
	base := strings.TrimSpace(name)
	if base == "" {
		base = "question"
	}
	return base + "-figure.png"
}

const (
	// AI 给出的图形框常常贴得过紧，直接裁剪会把图形边缘的线条切掉。
	// 这里先把框扩展到与之相连的完整图形，再在不含墨迹的空白处留出白边。
	cropPadPx       = 6
	cropSearchRatio = 0.2
	cropSearchMinPx = 64
	cropInkLuma     = 210
	cropInkFraction = 0.004
)

func growCropRect(img image.Image, rect image.Rectangle) image.Rectangle {
	bounds := img.Bounds()
	rect = rect.Intersect(bounds)
	if rect.Empty() {
		return rect
	}
	rect = expandToFigure(img, rect)
	return padBlankSpace(img, rect, bounds)
}

// expandToFigure 把矩形扩展到覆盖与它相交的每个墨迹连通块，
// 这样被 AI 框切开的图形线条会完整保留，而相邻的文字（独立连通块）不会被带进来。
func expandToFigure(img image.Image, rect image.Rectangle) image.Rectangle {
	bounds := img.Bounds()
	marginX := max(int(math.Round(float64(bounds.Dx())*cropSearchRatio)), cropSearchMinPx)
	marginY := max(int(math.Round(float64(bounds.Dy())*cropSearchRatio)), cropSearchMinPx)
	win := image.Rect(rect.Min.X-marginX, rect.Min.Y-marginY, rect.Max.X+marginX, rect.Max.Y+marginY).Intersect(bounds)
	w, h := win.Dx(), win.Dy()
	if w <= 0 || h <= 0 {
		return rect
	}

	ink := make([]bool, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			ink[y*w+x] = luma(img, win.Min.X+x, win.Min.Y+y) < cropInkLuma
		}
	}

	seen := make([]bool, w*h)
	stack := make([]int32, 0, 1024)
	out := rect
	for start := 0; start < len(ink); start++ {
		if !ink[start] || seen[start] {
			continue
		}
		seen[start] = true
		stack = append(stack[:0], int32(start))
		box := image.Rect(start%w, start/w, start%w+1, start/w+1)
		for len(stack) > 0 {
			cur := int(stack[len(stack)-1])
			stack = stack[:len(stack)-1]
			cx, cy := cur%w, cur/w
			if cx < box.Min.X {
				box.Min.X = cx
			}
			if cy < box.Min.Y {
				box.Min.Y = cy
			}
			if cx+1 > box.Max.X {
				box.Max.X = cx + 1
			}
			if cy+1 > box.Max.Y {
				box.Max.Y = cy + 1
			}
			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					nx, ny := cx+dx, cy+dy
					if nx < 0 || ny < 0 || nx >= w || ny >= h {
						continue
					}
					ni := ny*w + nx
					if !ink[ni] || seen[ni] {
						continue
					}
					seen[ni] = true
					stack = append(stack, int32(ni))
				}
			}
		}
		box = box.Add(win.Min)
		if box.Overlaps(rect) {
			out = out.Union(box)
		}
	}
	return out.Intersect(bounds)
}

func padBlankSpace(img image.Image, rect, bounds image.Rectangle) image.Rectangle {
	for i := 0; i < cropPadPx && rect.Max.X < bounds.Max.X; i++ {
		if hasInkColumn(img, rect.Max.X, rect.Min.Y, rect.Max.Y) {
			break
		}
		rect.Max.X++
	}
	for i := 0; i < cropPadPx && rect.Min.X > bounds.Min.X; i++ {
		if hasInkColumn(img, rect.Min.X-1, rect.Min.Y, rect.Max.Y) {
			break
		}
		rect.Min.X--
	}
	for i := 0; i < cropPadPx && rect.Max.Y < bounds.Max.Y; i++ {
		if hasInkRow(img, rect.Max.Y, rect.Min.X, rect.Max.X) {
			break
		}
		rect.Max.Y++
	}
	for i := 0; i < cropPadPx && rect.Min.Y > bounds.Min.Y; i++ {
		if hasInkRow(img, rect.Min.Y-1, rect.Min.X, rect.Max.X) {
			break
		}
		rect.Min.Y--
	}
	return rect
}

func hasInkColumn(img image.Image, x, y0, y1 int) bool {
	return countInk(y1-y0, func(i int) int { return luma(img, x, y0+i) })
}

func hasInkRow(img image.Image, y, x0, x1 int) bool {
	return countInk(x1-x0, func(i int) int { return luma(img, x0+i, y) })
}

func countInk(n int, at func(int) int) bool {
	if n <= 0 {
		return false
	}
	need := max(int(math.Round(float64(n)*cropInkFraction)), 1)
	ink := 0
	for i := 0; i < n; i++ {
		if at(i) < cropInkLuma {
			ink++
			if ink >= need {
				return true
			}
		}
	}
	return false
}

func luma(img image.Image, x, y int) int {
	r, g, b, _ := img.At(x, y).RGBA()
	return int((299*float64(r>>8) + 587*float64(g>>8) + 114*float64(b>>8)) / 1000)
}

func imageCropRect(bounds image.Rectangle, box ai.ImageBox) image.Rectangle {
	x := clamp01(box.X)
	y := clamp01(box.Y)
	w := clamp01(box.W)
	h := clamp01(box.H)
	if w <= 0 || h <= 0 {
		return image.Rectangle{}
	}
	if x+w > 1 {
		w = 1 - x
	}
	if y+h > 1 {
		h = 1 - y
	}
	width, height := bounds.Dx(), bounds.Dy()
	x0 := bounds.Min.X + int(math.Round(x*float64(width)))
	y0 := bounds.Min.Y + int(math.Round(y*float64(height)))
	x1 := bounds.Min.X + int(math.Round((x+w)*float64(width)))
	y1 := bounds.Min.Y + int(math.Round((y+h)*float64(height)))
	if x0 < bounds.Min.X {
		x0 = bounds.Min.X
	}
	if y0 < bounds.Min.Y {
		y0 = bounds.Min.Y
	}
	if x1 > bounds.Max.X {
		x1 = bounds.Max.X
	}
	if y1 > bounds.Max.Y {
		y1 = bounds.Max.Y
	}
	return image.Rect(x0, y0, x1, y1)
}

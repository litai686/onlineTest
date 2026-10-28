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

package filex

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/ledongthuc/pdf"
)

var ErrUnsupportedType = errors.New("unsupported file type")
var ErrTooLarge = errors.New("file too large")

type Saved struct {
	ID           string
	OriginalName string
	Mime         string
	StoredPath   string
	SizeBytes    int64
	SHA256       string
	Width        int
	Height       int
}

func Save(dataDir string, fh *multipart.FileHeader, maxBytes int64) (*Saved, error) {
	if fh.Size > maxBytes {
		return nil, ErrTooLarge
	}
	f, err := fh.Open()
	if err != nil {
		return nil, err
	}
	defer f.Close()

	limited := io.LimitReader(f, maxBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, ErrTooLarge
	}

	mime := http.DetectContentType(data)
	var ext string
	width, height := 0, 0
	switch {
	case mime == "image/jpeg":
		ext = ".jpg"
		cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("invalid jpeg: %w", err)
		}
		width, height = cfg.Width, cfg.Height
	case mime == "image/png":
		ext = ".png"
		cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("invalid png: %w", err)
		}
		width, height = cfg.Width, cfg.Height
	case mime == "image/webp":
		ext = ".webp"
	case mime == "application/pdf":
		ext = ".pdf"
	default:
		return nil, ErrUnsupportedType
	}

	sum := sha256.Sum256(data)
	now := time.Now().UTC()
	rel := filepath.Join("uploads", now.Format("2006"), now.Format("01"), uuid.NewString()+ext)
	abs := filepath.Join(dataDir, rel)
	if err := os.MkdirAll(filepath.Dir(abs), 0o700); err != nil {
		return nil, err
	}
	if err := os.WriteFile(abs, data, 0o600); err != nil {
		return nil, err
	}
	return &Saved{
		ID:           uuid.NewString(),
		OriginalName: fh.Filename,
		Mime:         mime,
		StoredPath:   filepath.ToSlash(rel),
		SizeBytes:    int64(len(data)),
		SHA256:       hex.EncodeToString(sum[:]),
		Width:        width,
		Height:       height,
	}, nil
}

func ExtractPDFText(path string) (string, int, error) {
	f, r, err := pdf.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()

	var sb strings.Builder
	for i := 1; i <= r.NumPage(); i++ {
		p := r.Page(i)
		if p.V.IsNull() {
			continue
		}
		text, err := p.GetPlainText(nil)
		if err != nil {
			continue
		}
		sb.WriteString(text)
		sb.WriteString("\n")
	}
	return sb.String(), r.NumPage(), nil
}

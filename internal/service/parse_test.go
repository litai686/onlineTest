package service

import (
	"image"
	"testing"

	"onlineTest/internal/ai"
)

func TestNormalizeParsedQuestionsKeepsSourceImage(t *testing.T) {
	got := normalizeParsedQuestions([]ai.ParsedQuestion{{
		Seq:        1,
		Type:       "short",
		Stem:       `如图，四边形 $ABCD$ 的对角线交于点 $O$，求面积最小值。`,
		Score:      10,
		SourceFile: "file-abc",
		Confidence: 0.9,
	}})
	if len(got) != 1 {
		t.Fatalf("want 1 question, got %d", len(got))
	}
	if got[0].SourceImage != "file-abc" {
		t.Fatalf("want source image file-abc, got %q", got[0].SourceImage)
	}
}

func TestStripQuestionNumber(t *testing.T) {
	cases := map[string]string{
		`3. 如图，求面积`:    `如图，求面积`,
		`（3）如图，求面积`:    `如图，求面积`,
		`第 3 题：如图，求面积`: `如图，求面积`,
		`三、如图，求面积`:     `如图，求面积`,
		`如图，求面积`:       `如图，求面积`,
		`2024 年产量为`:    `2024 年产量为`,
	}
	for in, want := range cases {
		if got := stripQuestionNumber(in); got != want {
			t.Errorf("stripQuestionNumber(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestImageCropRect(t *testing.T) {
	bounds := image.Rect(0, 0, 100, 200)
	got := imageCropRect(bounds, ai.ImageBox{X: 0.1, Y: 0.2, W: 0.5, H: 0.3})
	want := image.Rect(10, 40, 60, 100)
	if got != want {
		t.Fatalf("got %v, want %v", got, want)
	}
	if got := imageCropRect(bounds, ai.ImageBox{X: 0.9, Y: 0.9, W: 0.5, H: 0.5}); got != image.Rect(90, 180, 100, 200) {
		t.Fatalf("clamped box = %v", got)
	}
}

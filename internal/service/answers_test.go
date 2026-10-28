package service

import (
	"testing"

	"onlineTest/internal/domain"
)

func TestNormalizeAIAnswer(t *testing.T) {
	opts := domain.OptionList{{Key: "A", Text: "2"}, {Key: "B", Text: "3"}}
	tests := []struct {
		name string
		q    domain.Question
		in   []string
		want domain.StringList
		ok   bool
	}{
		{"single key", domain.Question{Type: "single", Options: opts}, []string{"a"}, domain.StringList{"A"}, true},
		{"single text", domain.Question{Type: "single", Options: opts}, []string{"2"}, domain.StringList{"A"}, true},
		{"single multi", domain.Question{Type: "single", Options: opts}, []string{"A", "B"}, nil, false},
		{"multiple", domain.Question{Type: "multiple", Options: opts}, []string{"b", "a"}, domain.StringList{"B", "A"}, true},
		{"judge", domain.Question{Type: "judge"}, []string{"正确"}, domain.StringList{"true"}, true},
		{"fill", domain.Question{Type: "fill"}, []string{" 4 "}, domain.StringList{"4"}, true},
		{"short", domain.Question{Type: "short"}, []string{"x=1"}, domain.StringList{"x=1"}, true},
		{"empty", domain.Question{Type: "single", Options: opts}, nil, nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := normalizeAIAnswer(tt.q, tt.in)
			if ok != tt.ok {
				t.Fatalf("ok = %v, want %v", ok, tt.ok)
			}
			if ok && len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("got %v, want %v", got, tt.want)
				}
			}
		})
	}
}

func TestQuestionSignature(t *testing.T) {
	a := domain.Question{Type: "single", Stem: " 1 + 1 = （ ） ", Options: domain.OptionList{{Key: "A", Text: "2"}, {Key: "B", Text: "3"}}}
	b := domain.Question{Type: "single", Stem: "1+1=()", Options: domain.OptionList{{Key: "A", Text: "2 "}, {Key: "B", Text: "3"}}}
	c := domain.Question{Type: "single", Stem: "1+1=()", Options: domain.OptionList{{Key: "A", Text: "2"}, {Key: "B", Text: "4"}}}
	if questionSignature(a) != questionSignature(b) {
		t.Fatal("equivalent questions should share signature")
	}
	if questionSignature(a) == questionSignature(c) {
		t.Fatal("different options must not share signature")
	}
}

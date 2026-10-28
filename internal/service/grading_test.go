package service

import (
	"testing"

	"onlineTest/internal/domain"
)

func TestNormalizeAnswer(t *testing.T) {
	cases := map[string]string{
		" $\\frac{1}{2}$ ":   `\frac{1}{2}`,
		"\\left( x \\right)": "(x)",
		"\\dfrac{3}{4}":      `\frac{3}{4}`,
		"ＡＢＣ":                "abc",
		"  2  +  3 ":         "2 + 3",
		"x^{-1}":             "x^{-1}",
		"对":                  "对",
	}
	for in, want := range cases {
		if got := normalizeAnswer(in); got != want {
			t.Errorf("normalizeAnswer(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAnswersEqual(t *testing.T) {
	tests := []struct {
		name    string
		q       questionForTest
		student []string
		want    bool
	}{
		{"single case", questionForTest{"single", []string{"A"}}, []string{"a"}, true},
		{"single wrong", questionForTest{"single", []string{"A"}}, []string{"B"}, false},
		{"multiple order", questionForTest{"multiple", []string{"A", "C"}}, []string{"c", "a"}, true},
		{"multiple subset", questionForTest{"multiple", []string{"A", "C"}}, []string{"A"}, false},
		{"judge true", questionForTest{"judge", []string{"true"}}, []string{"对"}, true},
		{"judge false", questionForTest{"judge", []string{"true"}}, []string{"false"}, false},
		{"fill latex", questionForTest{"fill", []string{"\\frac{5}{6}"}}, []string{"$\\dfrac{5}{6}$"}, true},
		{"fill blank count", questionForTest{"fill", []string{"2", "3"}}, []string{"2"}, false},
		{"empty", questionForTest{"single", []string{"A"}}, nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := questionFromTest(tt.q)
			if got := answersEqual(q, tt.student); got != tt.want {
				t.Fatalf("answersEqual = %v, want %v", got, tt.want)
			}
		})
	}
}

type questionForTest struct {
	typ    string
	answer []string
}

func questionFromTest(q questionForTest) domain.Question {
	return domain.Question{Type: q.typ, Answer: q.answer}
}

func TestClampScore(t *testing.T) {
	if got := clampScore(7.3, 10); got != 7.5 {
		t.Fatalf("clampScore(7.3) = %v", got)
	}
	if got := clampScore(-1, 10); got != 0 {
		t.Fatalf("clampScore(-1) = %v", got)
	}
	if got := clampScore(12, 10); got != 10 {
		t.Fatalf("clampScore(12) = %v", got)
	}
}

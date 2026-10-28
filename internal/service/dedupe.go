package service

import (
	"sort"
	"strings"
	"unicode"

	"onlineTest/internal/domain"
)

func questionSignature(q domain.Question) string {
	options := make([]string, 0, len(q.Options))
	for _, o := range q.Options {
		options = append(options, normalizeQuestionText(o.Key)+":"+normalizeQuestionText(o.Text))
	}
	sort.Strings(options)
	return q.Type + "|" + normalizeQuestionText(q.Stem) + "|" + strings.Join(options, ";")
}

func normalizeQuestionText(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, "$", "")
	s = strings.ReplaceAll(s, "\\left", "")
	s = strings.ReplaceAll(s, "\\right", "")
	s = strings.ReplaceAll(s, "\\dfrac", "\\frac")
	s = strings.ReplaceAll(s, "\\tfrac", "\\frac")
	var b strings.Builder
	for _, r := range s {
		if unicode.IsSpace(r) {
			continue
		}
		if strings.ContainsRune("，。；：！？、,.!?;:（）()【】[]{}<>《》\"'“”‘’", r) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

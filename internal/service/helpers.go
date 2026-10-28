package service

import (
	"context"
	"encoding/json"
	"errors"

	"onlineTest/internal/ai"
	"onlineTest/internal/domain"
)

var ErrNotFound = errors.New("not found")

func unmarshalPayload(raw string, v any) error {
	if raw == "" {
		raw = "{}"
	}
	return json.Unmarshal([]byte(raw), v)
}

func jsonUnmarshalFlex(raw []byte, out any) error {
	if err := json.Unmarshal(raw, out); err == nil {
		return nil
	}
	switch v := out.(type) {
	case *[]ai.GradeItem:
		var w struct {
			Items   []ai.GradeItem `json:"items"`
			Grades  []ai.GradeItem `json:"grades"`
			Results []ai.GradeItem `json:"results"`
		}
		if err := json.Unmarshal(raw, &w); err == nil {
			switch {
			case len(w.Items) > 0:
				*v = w.Items
				return nil
			case len(w.Grades) > 0:
				*v = w.Grades
				return nil
			case len(w.Results) > 0:
				*v = w.Results
				return nil
			}
		}
		var single ai.GradeItem
		if err := json.Unmarshal(raw, &single); err == nil && single.QuestionID > 0 {
			*v = []ai.GradeItem{single}
			return nil
		}
	case *ai.EquivResult:
		var arr []ai.EquivItem
		if err := json.Unmarshal(raw, &arr); err == nil {
			v.Items = arr
			return nil
		}
	case *ai.ReportResult:
		var w struct {
			Report *ai.ReportResult `json:"report"`
		}
		if err := json.Unmarshal(raw, &w); err == nil && w.Report != nil {
			*v = *w.Report
			return nil
		}
	}
	return errors.New("无法解析 AI 返回的 JSON 结构")
}

type ReportPayload struct {
	AttemptID int64 `json:"attempt_id"`
}

func (s *Service) RunReportJob(ctx context.Context, job *domain.Job) error {
	var payload ReportPayload
	if err := unmarshalPayload(job.Payload, &payload); err != nil {
		return err
	}
	if payload.AttemptID == 0 {
		payload.AttemptID = job.RefID
	}
	return s.FinalizeAttempt(ctx, payload.AttemptID, "", true)
}

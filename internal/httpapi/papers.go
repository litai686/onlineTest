package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"onlineTest/internal/domain"
	"onlineTest/internal/service"
	"onlineTest/internal/store"
)

func questionDTO(q *domain.Question, withAnswer bool) map[string]any {
	options := q.Options
	if options == nil {
		options = domain.OptionList{}
	}
	knowledge := q.Knowledge
	if knowledge == nil {
		knowledge = domain.StringList{}
	}
	out := map[string]any{
		"id":            q.ID,
		"seq":           q.Seq,
		"type":          q.Type,
		"stem":          q.Stem,
		"options":       q.Options,
		"score":         q.Score,
		"difficulty":    q.Difficulty,
		"knowledge":     q.Knowledge,
		"grading_mode":  q.GradingMode,
		"need_manual":   q.NeedManual,
		"ai_confidence": q.AIConfidence,
		"analysis":      q.Analysis,
		"source_image":  q.SourceImage,
	}
	if withAnswer {
		answer := q.Answer
		if answer == nil {
			answer = domain.StringList{}
		}
		out["answer"] = answer
	}
	return out
}

func (a *App) listPapers(w http.ResponseWriter, r *http.Request) {
	page, size := pageParams(r)
	subjectID, _ := intQuery(r, "subject_id")
	rows, total, err := a.Store.ListPapers(r.Context(), store.PaperFilter{
		SubjectID: int64(subjectID),
		Stage:     strings.TrimSpace(r.URL.Query().Get("stage")),
		Status:    strings.TrimSpace(r.URL.Query().Get("status")),
		Keyword:   strings.TrimSpace(r.URL.Query().Get("keyword")),
		Page:      page,
		Size:      size,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": rows, "total": total, "page": page, "page_size": size})
}

type paperRequest struct {
	Title       string  `json:"title"`
	SubjectID   int64   `json:"subject_id"`
	Stage       string  `json:"stage"`
	Grade       string  `json:"grade"`
	Description string  `json:"description"`
	DurationMin int     `json:"duration_min"`
	AIGrading   *bool   `json:"ai_grading"`
	ShowAnswer  *bool   `json:"show_answer"`
	AllowRetry  *bool   `json:"allow_retry"`
	StrictTime  *bool   `json:"strict_time"`
	Status      *string `json:"status"`
}

func (a *App) createPaper(w http.ResponseWriter, r *http.Request) {
	var req paperRequest
	if err := decodeJSON(w, r, &req, 256<<10); err != nil {
		writeError(w, r, err)
		return
	}
	title := strings.TrimSpace(req.Title)
	if title == "" || len([]rune(title)) > 120 {
		writeError(w, r, badRequest("试卷标题不能为空且不超过 120 字"))
		return
	}
	sub, err := a.Store.GetSubject(r.Context(), req.SubjectID)
	if err != nil {
		writeError(w, r, badRequest("请选择有效的科目"))
		return
	}
	stage := req.Stage
	if stage == "" {
		stage = sub.Stage
	}
	if stage != sub.Stage {
		writeError(w, r, badRequest("学段与科目不一致"))
		return
	}
	u := userFromRequest(r)
	p := &domain.Paper{
		Title:       title,
		SubjectID:   sub.ID,
		Stage:       stage,
		Grade:       strings.TrimSpace(req.Grade),
		Description: strings.TrimSpace(req.Description),
		SourceType:  "manual",
		Status:      "draft",
		ParseStatus: "none",
		DurationMin: req.DurationMin,
		AIGrading:   req.AIGrading == nil || *req.AIGrading,
		ShowAnswer:  req.ShowAnswer == nil || *req.ShowAnswer,
		AllowRetry:  req.AllowRetry != nil && *req.AllowRetry,
		StrictTime:  req.StrictTime != nil && *req.StrictTime,
		CreatedBy:   u.ID,
	}
	if err := a.Store.CreatePaper(r.Context(), p); err != nil {
		writeError(w, r, err)
		return
	}
	p.Subject = sub
	writeJSON(w, http.StatusCreated, a.paperDetailData(r, p, true))
}

func (a *App) paperDetailData(r *http.Request, p *domain.Paper, withAnswer bool) map[string]any {
	files := make([]map[string]any, 0, len(p.Files))
	for _, pf := range p.Files {
		item := map[string]any{"id": pf.FileID, "role": pf.Role, "page_no": pf.PageNo}
		if pf.File != nil {
			item["original_name"] = pf.File.OriginalName
			item["mime"] = pf.File.Mime
			item["url"] = "/api/v1/files/" + pf.File.ID
		} else {
			item["url"] = "/api/v1/files/" + pf.FileID
		}
		files = append(files, item)
	}
	questions := make([]map[string]any, 0, len(p.Questions))
	for i := range p.Questions {
		questions = append(questions, questionDTO(&p.Questions[i], withAnswer))
	}
	subject := map[string]any{"id": p.SubjectID, "name": ""}
	if p.Subject != nil {
		subject["name"] = p.Subject.Name
	}
	return map[string]any{
		"id":             p.ID,
		"title":          p.Title,
		"subject_id":     p.SubjectID,
		"subject":        subject,
		"stage":          p.Stage,
		"grade":          p.Grade,
		"description":    p.Description,
		"source_type":    p.SourceType,
		"status":         p.Status,
		"parse_status":   p.ParseStatus,
		"parse_error":    p.ParseError,
		"question_count": p.QuestionCount,
		"total_score":    p.TotalScore,
		"duration_min":   p.DurationMin,
		"ai_grading":     p.AIGrading,
		"show_answer":    p.ShowAnswer,
		"allow_retry":    p.AllowRetry,
		"strict_time":    p.StrictTime,
		"created_at":     p.CreatedAt,
		"published_at":   p.PublishedAt,
		"files":          files,
		"questions":      questions,
	}
}

func (a *App) getPaper(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, err)
		return
	}
	p, err := a.Store.GetPaper(r.Context(), id)
	if err != nil {
		writeError(w, r, notFound("试卷不存在"))
		return
	}
	data := a.paperDetailData(r, p, true)
	submitted, graded, avg, _ := a.Store.AttemptSummary(r.Context(), p.ID)
	data["stats"] = map[string]any{"attempts": submitted, "graded": graded, "avg_score": avg}
	writeJSON(w, http.StatusOK, data)
}

func (a *App) updatePaper(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, err)
		return
	}
	p, err := a.Store.GetPaperBasic(r.Context(), id)
	if err != nil {
		writeError(w, r, notFound("试卷不存在"))
		return
	}
	var req paperRequest
	if err := decodeJSON(w, r, &req, 256<<10); err != nil {
		writeError(w, r, err)
		return
	}
	fields := map[string]any{}
	if v := strings.TrimSpace(req.Title); v != "" {
		fields["title"] = v
	}
	if v := strings.TrimSpace(req.Grade); v != "" {
		fields["grade"] = v
	}
	if v := strings.TrimSpace(req.Description); v != "" {
		fields["description"] = v
	}
	if req.DurationMin > 0 {
		fields["duration_min"] = req.DurationMin
	}
	if req.AIGrading != nil {
		fields["ai_grading"] = *req.AIGrading
	}
	if req.ShowAnswer != nil {
		fields["show_answer"] = *req.ShowAnswer
	}
	if req.AllowRetry != nil {
		fields["allow_retry"] = *req.AllowRetry
	}
	if req.StrictTime != nil {
		fields["strict_time"] = *req.StrictTime
	}
	if req.SubjectID > 0 && req.SubjectID != p.SubjectID {
		if p.Status == "published" {
			writeError(w, r, invalidState("已发布试卷不能修改科目"))
			return
		}
		sub, err := a.Store.GetSubject(r.Context(), req.SubjectID)
		if err != nil {
			writeError(w, r, badRequest("科目不存在"))
			return
		}
		fields["subject_id"] = sub.ID
		fields["stage"] = sub.Stage
	}
	if len(fields) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}
	if err := a.Store.UpdatePaper(r.Context(), id, fields); err != nil {
		writeError(w, r, err)
		return
	}
	updated, _ := a.Store.GetPaper(r.Context(), id)
	writeJSON(w, http.StatusOK, a.paperDetailData(r, updated, true))
}

func (a *App) deletePaper(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, err)
		return
	}
	n, err := a.Store.CountAttemptsByPaper(r.Context(), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	if n > 0 {
		writeError(w, r, invalidState("该试卷已有作答记录，不能删除，可归档"))
		return
	}
	if err := a.Svc.DeletePaper(r.Context(), id); err != nil {
		writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) publishPaper(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, err)
		return
	}
	p, err := a.Store.GetPaperBasic(r.Context(), id)
	if err != nil {
		writeError(w, r, notFound("试卷不存在"))
		return
	}
	qlist, err := a.Store.ListQuestions(r.Context(), p.ID)
	if err != nil {
		writeError(w, r, err)
		return
	}

	problems := validateForPublish(p, qlist)
	if len(problems) > 0 {
		e := invalidState("试卷暂不满足发布条件")
		e.Data = map[string]any{"problems": problems}
		writeError(w, r, e)
		return
	}
	now := time.Now().UTC()
	if err := a.Store.UpdatePaper(r.Context(), p.ID, map[string]any{
		"status":       "published",
		"published_at": now,
	}); err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "published", "published_at": now})
}

func validateForPublish(p *domain.Paper, qs []domain.Question) []string {
	var problems []string
	if len(qs) == 0 {
		problems = append(problems, "试卷还没有题目")
		return problems
	}
	for _, q := range qs {
		if q.Score <= 0 {
			problems = append(problems, fmt.Sprintf("第 %d 题分值必须大于 0", q.Seq))
		}
		if len(q.Answer) == 0 && q.GradingMode != "manual" {
			problems = append(problems, fmt.Sprintf("第 %d 题缺少答案，请补充或设为人工阅卷", q.Seq))
		}
		if (q.Type == "single" || q.Type == "multiple") && len(q.Options) < 2 {
			problems = append(problems, fmt.Sprintf("第 %d 题选项不足", q.Seq))
		}
	}
	if len(problems) > 12 {
		problems = problems[:12]
	}
	return problems
}

func (a *App) archivePaper(w http.ResponseWriter, r *http.Request) {
	a.setPaperStatus(w, r, "archived")
}

func (a *App) restorePaper(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, err)
		return
	}
	p, err := a.Store.GetPaperBasic(r.Context(), id)
	if err != nil {
		writeError(w, r, notFound("试卷不存在"))
		return
	}
	status := "ready"
	if p.PublishedAt != nil {
		status = "published"
	}
	if err := a.Store.UpdatePaper(r.Context(), id, map[string]any{"status": status}); err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": status})
}

func (a *App) setPaperStatus(w http.ResponseWriter, r *http.Request, status string) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, err)
		return
	}
	if _, err := a.Store.GetPaperBasic(r.Context(), id); err != nil {
		writeError(w, r, notFound("试卷不存在"))
		return
	}
	if err := a.Store.UpdatePaper(r.Context(), id, map[string]any{"status": status}); err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": status})
}

type parseRequest struct {
	ProviderID int64
	Hint       string
	Mode       string
}

func (a *App) parsePaper(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, err)
		return
	}
	p, err := a.Store.GetPaperBasic(r.Context(), id)
	if err != nil {
		writeError(w, r, notFound("试卷不存在"))
		return
	}
	mode := r.FormValue("mode")
	if mode != "append" {
		mode = "replace"
	}
	if p.Status == "archived" {
		writeError(w, r, invalidState("已归档的试卷不能上传题目"))
		return
	}
	if p.Status == "published" && mode != "append" {
		writeError(w, r, invalidState("已发布试卷只能追加题目，不能替换全部"))
		return
	}
	if existing, err := a.Store.FindActiveJob(r.Context(), "paper_parse", "paper", p.ID); err == nil {
		e := conflict("该试卷已有解析任务在进行")
		e.Data = map[string]any{"job_id": existing.ID}
		writeError(w, r, e)
		return
	}

	maxBytes := a.Cfg.UploadMB << 20
	totalLimit := maxBytes*30 + 8<<20
	if totalLimit > 200<<20 {
		totalLimit = 200 << 20
	}
	r.Body = http.MaxBytesReader(w, r.Body, totalLimit)
	if err := r.ParseMultipartForm(maxBytes); err != nil {
		writeError(w, r, badRequest("上传失败："+err.Error()))
		return
	}
	fhs := r.MultipartForm.File["files"]
	if len(fhs) == 0 {
		writeError(w, r, badRequest("请上传试卷图片或 PDF"))
		return
	}
	if len(fhs) > 30 {
		writeError(w, r, badRequest("单次最多上传 30 个文件；更多页面请分批追加"))
		return
	}

	pageOffset := 0
	existingHashes := map[string]bool{}
	if mode == "append" {
		existing, err := a.Store.PaperFiles(r.Context(), p.ID)
		if err != nil {
			writeError(w, r, err)
			return
		}
		for _, pf := range existing {
			if pf.PageNo > pageOffset {
				pageOffset = pf.PageNo
			}
		}
		hashes, err := a.Store.PaperFileHashes(r.Context(), p.ID)
		if err != nil {
			writeError(w, r, err)
			return
		}
		if p.ParseStatus != "failed" {
			existingHashes = hashes
		}
	}

	u := userFromRequest(r)
	var paperFiles []domain.PaperFile
	var fileIDs []string
	sourceType := "image"
	skippedFiles := 0
	batchHashes := map[string]bool{}
	for _, fh := range fhs {
		f, err := a.Svc.SaveUpload(r.Context(), u.ID, fh, "paper_source")
		if err != nil {
			writeError(w, r, err)
			return
		}
		if batchHashes[f.SHA256] || (mode == "append" && existingHashes[f.SHA256]) {
			a.Svc.RemoveUpload(r.Context(), f)
			skippedFiles++
			continue
		}
		batchHashes[f.SHA256] = true
		if f.Mime == "application/pdf" {
			sourceType = "pdf"
		}
		paperFiles = append(paperFiles, domain.PaperFile{PaperID: p.ID, FileID: f.ID, Role: "source", PageNo: pageOffset + len(paperFiles) + 1})
		fileIDs = append(fileIDs, f.ID)
	}
	if len(fileIDs) == 0 {
		e := conflict("所选文件均已上传过，未重复解析")
		e.Data = map[string]any{"skipped_files": skippedFiles}
		writeError(w, r, e)
		return
	}
	if mode == "replace" {
		if err := a.Svc.DeletePaperSourceFiles(r.Context(), p.ID); err != nil {
			writeError(w, r, err)
			return
		}
		if err := a.Store.ReplacePaperFiles(r.Context(), p.ID, paperFiles); err != nil {
			writeError(w, r, err)
			return
		}
	} else if err := a.Store.AppendPaperFiles(r.Context(), p.ID, paperFiles); err != nil {
		writeError(w, r, err)
		return
	}

	providerID, _ := strconv.ParseInt(r.FormValue("provider_id"), 10, 64)
	job, err := a.Svc.EnqueueJob(r.Context(), "paper_parse", "paper", p.ID, map[string]any{
		"file_ids":      fileIDs,
		"provider_id":   providerID,
		"hint":          r.FormValue("hint"),
		"mode":          mode,
		"skipped_files": skippedFiles,
	}, u.ID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	_ = a.Store.UpdatePaperParseStatus(r.Context(), p.ID, "queued", "")
	updates := map[string]any{}
	if mode == "replace" || p.SourceType == "manual" {
		updates["source_type"] = sourceType
	}
	if mode == "replace" {
		updates["status"] = "draft"
	}
	if len(updates) > 0 {
		_ = a.Store.UpdatePaper(r.Context(), p.ID, updates)
	}
	writeJSON(w, http.StatusAccepted, map[string]any{
		"job_id":        job.ID,
		"paper_id":      p.ID,
		"parse_status":  "queued",
		"mode":          mode,
		"files":         len(fileIDs),
		"skipped_files": skippedFiles,
	})
}

type questionRequest struct {
	Type        string            `json:"type"`
	Stem        string            `json:"stem"`
	Options     domain.OptionList `json:"options"`
	Answer      domain.StringList `json:"answer"`
	Score       *float64          `json:"score"`
	Analysis    string            `json:"analysis"`
	Knowledge   domain.StringList `json:"knowledge"`
	Difficulty  string            `json:"difficulty"`
	GradingMode string            `json:"grading_mode"`
	NeedManual  *bool             `json:"need_manual"`
}

func validateQuestionInput(req questionRequest) error {
	typ := normalizeQuestionType(req.Type)
	if typ == "" {
		return badRequest("题型无效")
	}
	if strings.TrimSpace(req.Stem) == "" {
		return badRequest("题干不能为空")
	}
	if req.Score == nil || *req.Score <= 0 {
		return badRequest("分值必须大于 0")
	}
	if typ == "single" || typ == "multiple" {
		if len(req.Options) < 2 {
			return badRequest("选择题至少需要 2 个选项")
		}
		for _, o := range req.Options {
			if strings.TrimSpace(o.Key) == "" {
				return badRequest("选项缺少标识")
			}
		}
	}
	if req.GradingMode != "" && req.GradingMode != "auto" && req.GradingMode != "ai" && req.GradingMode != "manual" {
		return badRequest("阅卷方式无效")
	}
	return nil
}

func normalizeQuestionType(t string) string {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "single", "multiple", "judge", "fill", "short":
		return strings.ToLower(strings.TrimSpace(t))
	}
	return ""
}

func applyQuestionRequest(q *domain.Question, req questionRequest) {
	q.Type = normalizeQuestionType(req.Type)
	q.Stem = strings.TrimSpace(req.Stem)
	q.Options = req.Options
	q.Answer = req.Answer
	if req.Score != nil {
		q.Score = *req.Score
	}
	q.Analysis = strings.TrimSpace(req.Analysis)
	q.Knowledge = req.Knowledge
	q.Difficulty = strings.TrimSpace(req.Difficulty)
	if q.Difficulty == "" {
		q.Difficulty = "medium"
	}
	q.GradingMode = req.GradingMode
	if q.GradingMode == "" {
		q.GradingMode = "auto"
	}
	if req.NeedManual != nil {
		q.NeedManual = *req.NeedManual
	}
}

func (a *App) createQuestion(w http.ResponseWriter, r *http.Request) {
	paperID, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, err)
		return
	}
	if _, err := a.Store.GetPaperBasic(r.Context(), paperID); err != nil {
		writeError(w, r, notFound("试卷不存在"))
		return
	}
	var req questionRequest
	if err := decodeJSON(w, r, &req, 512<<10); err != nil {
		writeError(w, r, err)
		return
	}
	if err := validateQuestionInput(req); err != nil {
		writeError(w, r, err)
		return
	}
	q := &domain.Question{PaperID: paperID}
	applyQuestionRequest(q, req)
	if err := a.Store.CreateQuestion(r.Context(), q); err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, questionDTO(q, true))
}

func (a *App) updateQuestion(w http.ResponseWriter, r *http.Request) {
	paperID, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, err)
		return
	}
	qid, err := pathID(r, "qid")
	if err != nil {
		writeError(w, r, err)
		return
	}
	q, err := a.Store.GetQuestion(r.Context(), qid)
	if err != nil || q.PaperID != paperID {
		writeError(w, r, notFound("题目不存在"))
		return
	}
	var req questionRequest
	if err := decodeJSON(w, r, &req, 512<<10); err != nil {
		writeError(w, r, err)
		return
	}
	if err := validateQuestionInput(req); err != nil {
		writeError(w, r, err)
		return
	}
	merged := *q
	applyQuestionRequest(&merged, req)
	fields := map[string]any{
		"type":         merged.Type,
		"stem":         merged.Stem,
		"options":      merged.Options,
		"answer":       merged.Answer,
		"score":        merged.Score,
		"analysis":     merged.Analysis,
		"knowledge":    merged.Knowledge,
		"difficulty":   merged.Difficulty,
		"grading_mode": merged.GradingMode,
		"need_manual":  merged.NeedManual,
	}
	if err := a.Store.UpdateQuestion(r.Context(), qid, fields); err != nil {
		writeError(w, r, err)
		return
	}
	updated, _ := a.Store.GetQuestion(r.Context(), qid)
	writeJSON(w, http.StatusOK, questionDTO(updated, true))
}

func (a *App) deleteQuestion(w http.ResponseWriter, r *http.Request) {
	paperID, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, err)
		return
	}
	qid, err := pathID(r, "qid")
	if err != nil {
		writeError(w, r, err)
		return
	}
	q, err := a.Store.GetQuestion(r.Context(), qid)
	if err != nil || q.PaperID != paperID {
		writeError(w, r, notFound("题目不存在"))
		return
	}
	if err := a.Svc.DeleteQuestion(r.Context(), qid); err != nil {
		if errors.Is(err, store.ErrQuestionInUse) {
			writeError(w, r, invalidState("这道题已有学生提交的答卷，不能删除"))
			return
		}
		writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) aiAnswerQuestion(w http.ResponseWriter, r *http.Request) {
	paperID, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, err)
		return
	}
	qid, err := pathID(r, "qid")
	if err != nil {
		writeError(w, r, err)
		return
	}
	q, err := a.Store.GetQuestion(r.Context(), qid)
	if err != nil || q.PaperID != paperID {
		writeError(w, r, notFound("题目不存在"))
		return
	}
	previous := q.Answer
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Minute)
	defer cancel()
	updated, changed, feedback, err := a.Svc.ReviewQuestionAnswer(ctx, paperID, qid)
	if err != nil {
		if err == service.ErrNotFound {
			writeError(w, r, notFound("题目不存在"))
			return
		}
		writeError(w, r, aiError("AI 作答失败："+err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"question":        questionDTO(updated, true),
		"changed":         changed,
		"feedback":        feedback,
		"previous_answer": previous,
	})
}

func (a *App) reorderQuestions(w http.ResponseWriter, r *http.Request) {
	paperID, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, err)
		return
	}
	var req struct {
		QuestionIDs []int64 `json:"question_ids"`
	}
	if err := decodeJSON(w, r, &req, 64<<10); err != nil {
		writeError(w, r, err)
		return
	}
	if len(req.QuestionIDs) == 0 {
		writeError(w, r, badRequest("缺少题目顺序"))
		return
	}
	if err := a.Store.ReorderQuestions(r.Context(), paperID, req.QuestionIDs); err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

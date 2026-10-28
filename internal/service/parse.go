package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"onlineTest/internal/ai"
	"onlineTest/internal/domain"
	"onlineTest/internal/filex"
	"onlineTest/internal/store"
)

type ParsePayload struct {
	FileIDs      []string `json:"file_ids"`
	ProviderID   int64    `json:"provider_id"`
	Hint         string   `json:"hint"`
	Mode         string   `json:"mode"`
	SkippedFiles int      `json:"skipped_files"`
}

func (s *Service) RunParseJob(ctx context.Context, job *domain.Job) error {
	var payload ParsePayload
	if job.Payload != "" {
		if err := json.Unmarshal([]byte(job.Payload), &payload); err != nil {
			return fmt.Errorf("任务参数错误: %w", err)
		}
	}
	paper, err := s.Store.GetPaperBasic(ctx, job.RefID)
	if err != nil {
		return fmt.Errorf("试卷不存在: %w", err)
	}
	_ = s.Store.UpdatePaperParseStatus(ctx, paper.ID, "running", "")
	s.ReportProgress(ctx, job.ID, 5, "读取试卷文件")

	var files []domain.PaperFile
	if len(payload.FileIDs) > 0 {
		for i, id := range payload.FileIDs {
			files = append(files, domain.PaperFile{PaperID: paper.ID, FileID: id, Role: "source", PageNo: i + 1})
		}
	} else {
		files, err = s.Store.PaperFiles(ctx, paper.ID)
		if err != nil {
			return err
		}
	}

	var (
		images   []imgPart
		pdfText  string
		pageNum  int
		sourceTy = "image"
	)
	for _, pf := range files {
		f, err := s.Store.GetFile(ctx, pf.FileID)
		if err != nil {
			return fmt.Errorf("文件 %s 不存在: %w", pf.FileID, err)
		}
		if f.Mime == "application/pdf" {
			sourceTy = "pdf"
			text, pages, err := filex.ExtractPDFText(s.FilePath(f))
			if err != nil {
				return fmt.Errorf("解析 PDF 失败: %w", err)
			}
			if strings.TrimSpace(text) == "" {
				return fmt.Errorf("扫描版 PDF 无可提取文字，请将页面截图后以图片上传")
			}
			pdfText += text + "\n"
			pageNum += pages
			continue
		}
		data, err := os.ReadFile(s.FilePath(f))
		if err != nil {
			return fmt.Errorf("读取图片失败: %w", err)
		}
		images = append(images, imgPart{
			Mime: f.Mime,
			Data: base64.StdEncoding.EncodeToString(data),
		})
		pageNum++
	}
	if len(images) == 0 && strings.TrimSpace(pdfText) == "" {
		return fmt.Errorf("没有可解析的试卷文件")
	}

	provider, err := s.parseProvider(ctx, payload.ProviderID)
	if err != nil {
		return err
	}
	if len(images) > 0 && !provider.SupportsVision {
		return fmt.Errorf("当前识别模型不支持图片（未开启视觉能力），请更换模型或改用文本型 PDF")
	}

	parsed := ai.ParsedPaper{}
	perReq := pagesPerRequest(ctx, s.Store)
	batches := chunkImages(images, perReq)
	total := len(batches)
	if total == 0 {
		total = 1
	}

	callAI := func(parts []ai.ContentPart, batchName string) (*ai.ParsedPaper, error) {
		msgs := ai.ParseMessages(payload.Hint, pageNum, "")
		msgs[1].Content = parts
		res, err := s.AI.Chat(ctx, provider, ai.ChatRequest{
			Messages:       msgs,
			ResponseFormat: &ai.ResponseFormat{Type: "json_object"},
		}, "parse", job.ID)
		if err != nil {
			return nil, err
		}
		raw, err := ai.ExtractJSON(res.Content)
		if err != nil {
			return nil, fmt.Errorf("AI 返回无法解析为 JSON: %w", err)
		}
		var one ai.ParsedPaper
		if err := json.Unmarshal(raw, &one); err != nil {
			return nil, fmt.Errorf("解析识别结果失败: %w", err)
		}
		_ = batchName
		return &one, nil
	}

	if len(batches) == 0 {
		text := pdfText
		if len([]rune(text)) > 80000 {
			text = string([]rune(text)[:80000])
		}
		parts := []ai.ContentPart{ai.TextPart(fmt.Sprintf("请识别以下试卷文本（共 %d 页）。\n%s", pageNum, text))}
		one, err := callAI(parts, "text")
		if err != nil {
			return err
		}
		parsed = *one
	} else {
		for i, batch := range batches {
			parts := []ai.ContentPart{ai.TextPart(fmt.Sprintf("这是试卷的第 %d 批页面（共 %d 页/批），请识别其中的全部题目。", i+1, pageNum))}
			for _, img := range batch {
				parts = append(parts, ai.ImagePart("data:"+img.Mime+";base64,"+img.Data))
			}
			one, err := callAI(parts, fmt.Sprintf("batch-%d", i+1))
			if err != nil {
				return fmt.Errorf("第 %d 批识别失败: %w", i+1, err)
			}
			if parsed.Title == "" {
				parsed.Title = one.Title
			}
			if parsed.Grade == "" {
				parsed.Grade = one.Grade
			}
			if parsed.DurationMin == 0 {
				parsed.DurationMin = one.DurationMin
			}
			parsed.Questions = append(parsed.Questions, one.Questions...)
			progress := 10 + (i+1)*80/total
			s.ReportProgress(ctx, job.ID, progress, fmt.Sprintf("已识别 %d/%d 批，累计 %d 题", i+1, total, len(parsed.Questions)))
		}
	}

	if len(parsed.Questions) == 0 {
		return fmt.Errorf("未识别到题目，请检查图片清晰度或更换识别模型")
	}

	questions := normalizeParsedQuestions(parsed.Questions)
	replace := payload.Mode != "append"
	paperMessage := ""

	added, skippedQuestions, err := s.SaveParsedPaper(ctx, paper, questions, replace, sourceTy, parsed)
	if err != nil {
		return err
	}
	updated, statErr := s.Store.GetPaperBasic(ctx, paper.ID)
	if statErr != nil {
		updated = paper
	}
	if replace {
		message := fmt.Sprintf("识别完成，共 %d 题，总分 %.1f", added, updated.TotalScore)
		if skippedQuestions > 0 {
			message += fmt.Sprintf("，跳过重复题 %d 道", skippedQuestions)
		}
		paperMessage = message
	} else {
		message := fmt.Sprintf("识别完成，新增 %d 题，试卷共 %d 题，总分 %.1f", added, updated.QuestionCount, updated.TotalScore)
		if skippedQuestions > 0 {
			message += fmt.Sprintf("，跳过重复题 %d 道", skippedQuestions)
		}
		paperMessage = message
	}
	if payload.SkippedFiles > 0 {
		paperMessage += fmt.Sprintf("，跳过重复文件 %d 个", payload.SkippedFiles)
	}
	s.ReportProgress(ctx, job.ID, 92, "题目已入库，正在解析缺失答案")
	filled, unresolved, fillErr := s.FillMissingAnswers(ctx, paper.ID)
	warn := ""
	switch {
	case fillErr != nil:
		warn = "AI 补全答案失败：" + fillErr.Error()
	case len(unresolved) > 0:
		warn = "部分题目未能解析出答案：" + strings.Join(unresolved, "、")
	}
	if err := s.DeletePaperSourceFiles(ctx, paper.ID); err != nil {
		slog.Warn("delete source files failed", "paper_id", paper.ID, "error", err)
	} else {
		paperMessage += "，原始文件已清理"
	}
	if filled > 0 {
		paperMessage += fmt.Sprintf("，AI 已补全 %d 道答案", filled)
	}
	if warn != "" {
		paperMessage += "；" + warn
		_ = s.Store.UpdatePaperParseStatus(ctx, paper.ID, "succeeded", warn)
	}
	s.ReportProgress(ctx, job.ID, 100, paperMessage)
	return nil
}

func (s *Service) SaveParsedPaper(ctx context.Context, paper *domain.Paper, questions []domain.Question, replace bool, sourceType string, parsed ai.ParsedPaper) (added int, skipped int, err error) {
	err = s.Store.WithTx(func(tx *store.Store) error {
		seen := make(map[string]bool, len(questions))
		if !replace {
			var existing []domain.Question
			if err := tx.DB().WithContext(ctx).Where("paper_id = ?", paper.ID).Find(&existing).Error; err != nil {
				return err
			}
			for i := range existing {
				seen[questionSignature(existing[i])] = true
			}
		}
		kept := make([]domain.Question, 0, len(questions))
		for i := range questions {
			sig := questionSignature(questions[i])
			if seen[sig] {
				skipped++
				continue
			}
			seen[sig] = true
			kept = append(kept, questions[i])
		}
		questions = kept
		added = len(questions)

		if replace {
			if err := tx.DB().WithContext(ctx).Where("paper_id = ?", paper.ID).Delete(&domain.Question{}).Error; err != nil {
				return err
			}
			for i := range questions {
				questions[i].Seq = i + 1
			}
		} else {
			var maxSeq int
			if err := tx.DB().WithContext(ctx).Model(&domain.Question{}).Where("paper_id = ?", paper.ID).
				Select("COALESCE(MAX(seq), 0)").Scan(&maxSeq).Error; err != nil {
				return err
			}
			for i := range questions {
				questions[i].Seq = maxSeq + i + 1
			}
		}
		for i := range questions {
			questions[i].PaperID = paper.ID
		}
		if len(questions) > 0 {
			if err := tx.DB().WithContext(ctx).CreateInBatches(&questions, 50).Error; err != nil {
				return err
			}
		}
		if err := tx.RecalcPaper(ctx, paper.ID); err != nil {
			return err
		}

		updates := map[string]any{
			"parse_status": "succeeded",
			"parse_error":  "",
		}
		if !(paper.Status == "published" && !replace) {
			updates["status"] = "ready"
		}
		if replace || paper.SourceType == "manual" || paper.SourceType == "" {
			updates["source_type"] = sourceType
		}
		if strings.TrimSpace(paper.Title) == "" && parsed.Title != "" {
			updates["title"] = parsed.Title
		}
		if strings.TrimSpace(paper.Grade) == "" && parsed.Grade != "" {
			updates["grade"] = parsed.Grade
		}
		if paper.DurationMin == 0 && parsed.DurationMin > 0 {
			updates["duration_min"] = parsed.DurationMin
		}
		return tx.DB().WithContext(ctx).Model(&domain.Paper{}).Where("id = ?", paper.ID).Updates(updates).Error
	})
	return added, skipped, err
}

type imgPart struct {
	Mime string
	Data string
}

func chunkImages(items []imgPart, size int) [][]imgPart {
	if size < 1 {
		size = 3
	}
	var out [][]imgPart
	for i := 0; i < len(items); i += size {
		end := i + size
		if end > len(items) {
			end = len(items)
		}
		out = append(out, items[i:end])
	}
	return out
}

func (s *Service) parseProvider(ctx context.Context, id int64) (*domain.AIProvider, error) {
	if id > 0 {
		p, err := s.Store.GetProvider(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("指定的识别模型不存在")
		}
		return p, nil
	}
	p, err := s.AI.ProviderFor(ctx, "vision")
	if err != nil {
		return nil, fmt.Errorf("未配置可用的识别模型，请先在 AI 模型配置中添加并绑定")
	}
	return p, nil
}

func normalizeParsedQuestions(in []ai.ParsedQuestion) []domain.Question {
	out := make([]domain.Question, 0, len(in))
	seq := 0
	for _, q := range in {
		typ := normalizeType(q.Type)
		stem := strings.TrimSpace(q.Stem)
		if stem == "" {
			continue
		}
		seq++
		score := q.Score
		if score <= 0 {
			switch typ {
			case "single", "multiple", "judge":
				score = 3
			case "fill":
				score = 4
			default:
				score = 8
			}
		}
		opts := make(domain.OptionList, 0, len(q.Options))
		for _, o := range q.Options {
			k := strings.TrimSpace(o.Key)
			if k == "" {
				continue
			}
			opts = append(opts, domain.Option{Key: k, Text: strings.TrimSpace(o.Text)})
		}
		ans := make(domain.StringList, 0, len(q.Answer))
		for _, a := range q.Answer {
			a = strings.TrimSpace(a)
			if a != "" {
				ans = append(ans, a)
			}
		}
		difficulty := strings.ToLower(strings.TrimSpace(q.Difficulty))
		if difficulty != "easy" && difficulty != "hard" {
			difficulty = "medium"
		}
		knowledge := make(domain.StringList, 0, len(q.Knowledge))
		for _, k := range q.Knowledge {
			if k = strings.TrimSpace(k); k != "" {
				knowledge = append(knowledge, k)
			}
		}
		mode := "auto"
		needManual := q.NeedManual || len(ans) == 0
		if typ == "short" && len(ans) == 0 {
			mode = "manual"
		}
		out = append(out, domain.Question{
			Seq:          seq,
			Type:         typ,
			Stem:         stem,
			Options:      opts,
			Answer:       ans,
			Score:        score,
			Analysis:     strings.TrimSpace(q.Analysis),
			Knowledge:    knowledge,
			Difficulty:   difficulty,
			AIConfidence: clamp01(q.Confidence),
			NeedManual:   needManual,
			GradingMode:  mode,
		})
	}
	return out
}

func normalizeType(t string) string {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "single", "single_choice", "choice", "radio", "单选题":
		return "single"
	case "multiple", "multi", "multiple_choice", "checkbox", "多选题":
		return "multiple"
	case "judge", "true_false", "truefalse", "判断题":
		return "judge"
	case "fill", "blank", "fill_blank", "填空题":
		return "fill"
	default:
		return "short"
	}
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"onlineTest/internal/domain"
	"onlineTest/internal/store"
)

type Queue struct {
	svc      *Service
	st       *store.Store
	wake     chan struct{}
	stop     chan struct{}
	stopOnce sync.Once
	wg       sync.WaitGroup
}

func NewQueue(svc *Service) *Queue {
	return &Queue{
		svc:  svc,
		st:   svc.Store,
		wake: make(chan struct{}, 1),
		stop: make(chan struct{}),
	}
}

func (q *Queue) Start(workers int) {
	if workers < 1 {
		workers = 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := q.st.ResetRunningJobs(ctx); err != nil {
		slog.Warn("reset running jobs failed", "error", err)
	}
	for i := 0; i < workers; i++ {
		q.wg.Add(1)
		go q.worker(i)
	}
}

func (q *Queue) Stop() {
	q.stopOnce.Do(func() { close(q.stop) })
	q.wg.Wait()
}

func (q *Queue) Notify() {
	select {
	case q.wake <- struct{}{}:
	default:
	}
}

func (q *Queue) worker(id int) {
	defer q.wg.Done()
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-q.stop:
			return
		case <-q.wake:
		case <-ticker.C:
		}
		q.drain()
	}
}

func (q *Queue) drain() {
	for {
		select {
		case <-q.stop:
			return
		default:
		}
		job, err := q.st.ClaimNextJob(context.Background())
		if err != nil {
			if err != store.ErrNoPendingJob {
				slog.Error("claim job failed", "error", err)
			}
			return
		}
		q.execute(job)
	}
}

func (q *Queue) execute(job *domain.Job) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	slog.Info("job started", "job_id", job.ID, "type", job.Type, "ref", job.RefID)

	_ = q.st.UpdateJob(ctx, job.ID, map[string]any{"status": "running", "message": "处理中"})
	err := safeRun(ctx, job, q.svc)
	now := time.Now().UTC()
	if err != nil {
		slog.Error("job failed", "job_id", job.ID, "type", job.Type, "error", err)
		_ = q.st.UpdateJob(context.Background(), job.ID, map[string]any{
			"status":      "failed",
			"last_error":  truncateRunes(err.Error(), 500),
			"finished_at": now,
			"message":     "失败",
		})
		if job.Type == "paper_parse" {
			_ = q.st.UpdatePaperParseStatus(context.Background(), job.RefID, "failed", truncateRunes(err.Error(), 500))
		}
		return
	}
	fields := map[string]any{
		"status":      "succeeded",
		"progress":    100,
		"finished_at": now,
	}
	if current, err := q.st.GetJob(context.Background(), job.ID); err == nil && strings.TrimSpace(current.Message) != "" {
		fields["message"] = current.Message
	} else {
		fields["message"] = "完成"
	}
	_ = q.st.UpdateJob(context.Background(), job.ID, fields)
	slog.Info("job succeeded", "job_id", job.ID, "type", job.Type)
}

func safeRun(ctx context.Context, job *domain.Job, svc *Service) (err error) {
	defer func() {
		if rec := recover(); rec != nil {
			err = fmt.Errorf("panic: %v", rec)
			slog.Error("job panic", "job_id", job.ID, "type", job.Type, "panic", rec)
		}
	}()
	return svc.RunJob(ctx, job)
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

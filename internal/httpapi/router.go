package httpapi

import "net/http"

func (a *App) Routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/v1/healthz", a.health)
	mux.HandleFunc("GET /api/v1/version", a.version)

	mux.HandleFunc("POST /api/v1/auth/login", a.login)
	mux.HandleFunc("GET /api/v1/auth/me", a.authed(a.me))
	mux.HandleFunc("POST /api/v1/auth/change-password", a.authed(a.changePassword))

	mux.HandleFunc("GET /api/v1/subjects", a.authed(a.listSubjects))
	mux.HandleFunc("POST /api/v1/subjects", a.teacher(a.createSubject))
	mux.HandleFunc("PUT /api/v1/subjects/{id}", a.teacher(a.updateSubject))
	mux.HandleFunc("DELETE /api/v1/subjects/{id}", a.teacher(a.deleteSubject))

	mux.HandleFunc("GET /api/v1/students", a.teacher(a.listStudents))
	mux.HandleFunc("POST /api/v1/students", a.teacher(a.createStudent))
	mux.HandleFunc("PUT /api/v1/students/{id}", a.teacher(a.updateStudent))
	mux.HandleFunc("DELETE /api/v1/students/{id}", a.teacher(a.deleteStudent))
	mux.HandleFunc("POST /api/v1/students/{id}/reset-password", a.teacher(a.resetStudentPassword))

	mux.HandleFunc("GET /api/v1/ai/providers", a.teacher(a.listProviders))
	mux.HandleFunc("POST /api/v1/ai/providers", a.teacher(a.createProvider))
	mux.HandleFunc("PUT /api/v1/ai/providers/{id}", a.teacher(a.updateProvider))
	mux.HandleFunc("DELETE /api/v1/ai/providers/{id}", a.teacher(a.deleteProvider))
	mux.HandleFunc("POST /api/v1/ai/providers/{id}/test", a.teacher(a.testProvider))
	mux.HandleFunc("POST /api/v1/ai/roles", a.teacher(a.setRoles))
	mux.HandleFunc("GET /api/v1/ai/calls", a.teacher(a.listAICalls))

	mux.HandleFunc("POST /api/v1/files", a.authed(a.uploadFile))
	mux.HandleFunc("GET /api/v1/files/{id}", a.authed(a.downloadFile))

	mux.HandleFunc("GET /api/v1/papers", a.teacher(a.listPapers))
	mux.HandleFunc("POST /api/v1/papers", a.teacher(a.createPaper))
	mux.HandleFunc("GET /api/v1/papers/{id}", a.teacher(a.getPaper))
	mux.HandleFunc("PUT /api/v1/papers/{id}", a.teacher(a.updatePaper))
	mux.HandleFunc("DELETE /api/v1/papers/{id}", a.teacher(a.deletePaper))
	mux.HandleFunc("POST /api/v1/papers/{id}/publish", a.teacher(a.publishPaper))
	mux.HandleFunc("POST /api/v1/papers/{id}/archive", a.teacher(a.archivePaper))
	mux.HandleFunc("POST /api/v1/papers/{id}/restore", a.teacher(a.restorePaper))
	mux.HandleFunc("POST /api/v1/papers/{id}/parse", a.teacher(a.parsePaper))
	mux.HandleFunc("POST /api/v1/papers/{id}/questions", a.teacher(a.createQuestion))
	mux.HandleFunc("POST /api/v1/papers/{id}/questions/{qid}/ai-answer", a.teacher(a.aiAnswerQuestion))
	mux.HandleFunc("PUT /api/v1/papers/{id}/questions/order", a.teacher(a.reorderQuestions))
	mux.HandleFunc("PUT /api/v1/papers/{id}/questions/{qid}", a.teacher(a.updateQuestion))
	mux.HandleFunc("DELETE /api/v1/papers/{id}/questions/{qid}", a.teacher(a.deleteQuestion))

	mux.HandleFunc("GET /api/v1/jobs", a.teacher(a.listJobs))
	mux.HandleFunc("GET /api/v1/jobs/{id}", a.authed(a.getJob))
	mux.HandleFunc("POST /api/v1/jobs/{id}/retry", a.teacher(a.retryJob))
	mux.HandleFunc("POST /api/v1/jobs/{id}/cancel", a.teacher(a.cancelJob))

	mux.HandleFunc("GET /api/v1/student/papers", a.student(a.listStudentPapers))
	mux.HandleFunc("POST /api/v1/student/papers/{paperId}/attempts", a.student(a.startAttempt))
	mux.HandleFunc("GET /api/v1/student/attempts", a.student(a.studentHistory))
	mux.HandleFunc("GET /api/v1/student/attempts/{id}", a.student(a.getStudentAttempt))
	mux.HandleFunc("PUT /api/v1/student/attempts/{id}/answers", a.student(a.saveAnswers))
	mux.HandleFunc("PUT /api/v1/student/attempts/{id}/mode", a.student(a.setAttemptMode))
	mux.HandleFunc("POST /api/v1/student/attempts/{id}/questions/{qid}/check", a.student(a.checkPracticeQuestion))
	mux.HandleFunc("POST /api/v1/student/attempts/{id}/submit", a.student(a.submitAttempt))
	mux.HandleFunc("GET /api/v1/student/attempts/{id}/result", a.student(a.studentResult))

	mux.HandleFunc("GET /api/v1/teacher/attempts", a.teacher(a.teacherAttempts))
	mux.HandleFunc("GET /api/v1/teacher/attempts/{id}/result", a.teacher(a.teacherResult))
	mux.HandleFunc("PUT /api/v1/teacher/attempts/{id}/answers/{questionId}", a.teacher(a.manualScore))
	mux.HandleFunc("POST /api/v1/teacher/attempts/{id}/regrade", a.teacher(a.regradeAttempt))

	mux.HandleFunc("GET /api/v1/stats/overview", a.teacher(a.overview))

	mux.Handle("/", a.staticHandler())

	return a.middleware(mux)
}

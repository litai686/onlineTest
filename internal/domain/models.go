package domain

import (
	"time"

	"gorm.io/gorm"
)

type User struct {
	ID            int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	Username      string `gorm:"type:text;not null;uniqueIndex:ux_users_username" json:"username"`
	PasswordHash  string `gorm:"type:text;not null" json:"-"`
	DisplayName   string `gorm:"type:text;not null" json:"display_name"`
	Role          string `gorm:"type:text;not null;check:chk_users_role,role IN ('teacher','student')" json:"role"`
	ClassName     string `gorm:"type:text;not null;default:''" json:"class_name"`
	Grade         string `gorm:"type:text;not null;default:''" json:"grade"`
	Phone         string `gorm:"type:text;not null;default:''" json:"phone"`
	Status        int    `gorm:"not null;default:1" json:"status"`
	TokenVersion  int    `gorm:"not null;default:0" json:"-"`
	MustChangePwd bool   `gorm:"not null;default:false" json:"must_change_pwd"`
	LastLoginAt   *time.Time
	CreatedAt     time.Time      `gorm:"not null;autoCreateTime" json:"created_at"`
	UpdatedAt     time.Time      `gorm:"not null;autoUpdateTime" json:"updated_at"`
	DeletedAt     gorm.DeletedAt `gorm:"index:ix_users_deleted" json:"-"`
}

func (User) TableName() string { return "users" }

type Subject struct {
	ID         int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	Stage      string    `gorm:"type:text;not null;uniqueIndex:ux_subjects_stage_name,priority:1;check:chk_subjects_stage,stage IN ('junior','senior')" json:"stage"`
	Name       string    `gorm:"type:text;not null;uniqueIndex:ux_subjects_stage_name,priority:2" json:"name"`
	Code       string    `gorm:"type:text;not null;default:''" json:"code"`
	SortOrder  int       `gorm:"not null;default:0" json:"sort_order"`
	Status     int       `gorm:"not null;default:1" json:"status"`
	CreatedAt  time.Time `gorm:"not null;autoCreateTime" json:"created_at"`
	UpdatedAt  time.Time `gorm:"not null;autoUpdateTime" json:"updated_at"`
	PaperCount int64     `gorm:"-" json:"paper_count"`
}

func (Subject) TableName() string { return "subjects" }

type AIProvider struct {
	ID             int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	Name           string    `gorm:"type:text;not null" json:"name"`
	BaseURL        string    `gorm:"type:text;not null" json:"base_url"`
	APIKeyCipher   string    `gorm:"type:text;not null;default:''" json:"-"`
	APIKeyHint     string    `gorm:"type:text;not null;default:''" json:"api_key_hint"`
	Model          string    `gorm:"type:text;not null" json:"model"`
	SupportsVision bool      `gorm:"not null;default:false" json:"supports_vision"`
	SupportsJSON   bool      `gorm:"not null;default:true" json:"supports_json"`
	Temperature    float64   `gorm:"not null;default:0.2" json:"temperature"`
	MaxTokens      int       `gorm:"not null;default:4096" json:"max_tokens"`
	TimeoutSec     int       `gorm:"not null;default:120" json:"timeout_sec"`
	Concurrency    int       `gorm:"not null;default:2" json:"concurrency"`
	ThinkingMode   string    `gorm:"type:text;not null;default:''" json:"thinking_mode"`
	ExtraHeaders   string    `gorm:"type:text;not null;default:'{}'" json:"extra_headers"`
	ExtraBody      string    `gorm:"type:text;not null;default:'{}'" json:"extra_body"`
	Note           string    `gorm:"type:text;not null;default:''" json:"note"`
	Status         int       `gorm:"not null;default:1" json:"status"`
	CreatedAt      time.Time `gorm:"not null;autoCreateTime" json:"created_at"`
	UpdatedAt      time.Time `gorm:"not null;autoUpdateTime" json:"updated_at"`
}

func (AIProvider) TableName() string { return "ai_providers" }

type File struct {
	ID           string    `gorm:"type:text;primaryKey" json:"id"`
	OwnerID      *int64    `gorm:"index:ix_files_owner" json:"owner_id"`
	OriginalName string    `gorm:"type:text;not null" json:"original_name"`
	StoredPath   string    `gorm:"type:text;not null;uniqueIndex:ux_files_path" json:"-"`
	Mime         string    `gorm:"type:text;not null" json:"mime"`
	SizeBytes    int64     `gorm:"not null" json:"size_bytes"`
	SHA256       string    `gorm:"type:text;not null" json:"-"`
	Kind         string    `gorm:"type:text;not null;default:'paper_source';check:chk_files_kind,kind IN ('paper_source','question_image','avatar')" json:"kind"`
	Width        int       `gorm:"not null;default:0" json:"width"`
	Height       int       `gorm:"not null;default:0" json:"height"`
	CreatedAt    time.Time `gorm:"not null;autoCreateTime" json:"created_at"`
}

func (File) TableName() string { return "files" }

type Paper struct {
	ID            int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	Title         string    `gorm:"type:text;not null" json:"title"`
	SubjectID     int64     `gorm:"not null" json:"subject_id"`
	Subject       *Subject  `gorm:"foreignKey:SubjectID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"subject"`
	Stage         string    `gorm:"type:text;not null;check:chk_papers_stage,stage IN ('junior','senior')" json:"stage"`
	Grade         string    `gorm:"type:text;not null;default:''" json:"grade"`
	Description   string    `gorm:"type:text;not null;default:''" json:"description"`
	SourceType    string    `gorm:"type:text;not null;default:'manual';check:chk_papers_source,source_type IN ('manual','image','pdf')" json:"source_type"`
	Status        string    `gorm:"type:text;not null;default:'draft';check:chk_papers_status,status IN ('draft','ready','published','archived')" json:"status"`
	ParseStatus   string    `gorm:"type:text;not null;default:'none';check:chk_papers_parse,parse_status IN ('none','queued','running','succeeded','failed')" json:"parse_status"`
	ParseError    string    `gorm:"type:text;not null;default:''" json:"parse_error"`
	TotalScore    float64   `gorm:"not null;default:0" json:"total_score"`
	QuestionCount int       `gorm:"not null;default:0" json:"question_count"`
	DurationMin   int       `gorm:"not null;default:0" json:"duration_min"`
	AIGrading     bool      `gorm:"not null;default:true" json:"ai_grading"`
	ShowAnswer    bool      `gorm:"not null;default:true" json:"show_answer"`
	StrictTime    bool      `gorm:"not null;default:false" json:"strict_time"`
	AllowRetry    bool      `gorm:"not null;default:false" json:"allow_retry"`
	CreatedBy     int64     `gorm:"not null" json:"created_by"`
	CreatedByUser *User     `gorm:"foreignKey:CreatedBy;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"-"`
	CreatedAt     time.Time `gorm:"not null;autoCreateTime" json:"created_at"`
	UpdatedAt     time.Time `gorm:"not null;autoUpdateTime" json:"updated_at"`
	PublishedAt   *time.Time
	Questions     []Question  `gorm:"foreignKey:PaperID;constraint:OnDelete:CASCADE" json:"questions"`
	Files         []PaperFile `gorm:"foreignKey:PaperID;constraint:OnDelete:CASCADE" json:"files"`
}

func (Paper) TableName() string { return "papers" }

type PaperFile struct {
	PaperID int64  `gorm:"primaryKey;not null" json:"paper_id"`
	FileID  string `gorm:"primaryKey;type:text;not null" json:"file_id"`
	File    *File  `gorm:"foreignKey:FileID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"file"`
	Role    string `gorm:"type:text;not null;default:'source';check:chk_paper_files_role,role IN ('source','answer_sheet','question_image')" json:"role"`
	PageNo  int    `gorm:"not null;default:0" json:"page_no"`
}

func (PaperFile) TableName() string { return "paper_files" }

type Question struct {
	ID           int64      `gorm:"primaryKey;autoIncrement" json:"id"`
	PaperID      int64      `gorm:"not null;uniqueIndex:ux_questions_paper_seq,priority:1" json:"paper_id"`
	Seq          int        `gorm:"not null;uniqueIndex:ux_questions_paper_seq,priority:2" json:"seq"`
	Type         string     `gorm:"type:text;not null;check:chk_questions_type,type IN ('single','multiple','judge','fill','short')" json:"type"`
	Stem         string     `gorm:"type:text;not null" json:"stem"`
	Options      OptionList `gorm:"type:text;not null;default:'[]'" json:"options"`
	Answer       StringList `gorm:"type:text;not null;default:'[]'" json:"answer"`
	Score        float64    `gorm:"not null;default:5" json:"score"`
	Analysis     string     `gorm:"type:text;not null;default:''" json:"analysis"`
	Knowledge    StringList `gorm:"type:text;not null;default:'[]'" json:"knowledge"`
	Difficulty   string     `gorm:"type:text;not null;default:'medium';check:chk_questions_diff,difficulty IN ('easy','medium','hard')" json:"difficulty"`
	SourceImage  string     `gorm:"type:text;not null;default:''" json:"source_image"`
	AIConfidence float64    `gorm:"not null;default:0" json:"ai_confidence"`
	NeedManual   bool       `gorm:"not null;default:false" json:"need_manual"`
	GradingMode  string     `gorm:"type:text;not null;default:'auto';check:chk_questions_grading,grading_mode IN ('auto','ai','manual')" json:"grading_mode"`
	CreatedAt    time.Time  `gorm:"not null;autoCreateTime" json:"created_at"`
	UpdatedAt    time.Time  `gorm:"not null;autoUpdateTime" json:"updated_at"`
}

func (Question) TableName() string { return "questions" }

type Attempt struct {
	ID              int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	PaperID         int64  `gorm:"not null;uniqueIndex:ux_attempts_paper_student_no,priority:1" json:"paper_id"`
	Paper           *Paper `gorm:"foreignKey:PaperID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"paper"`
	StudentID       int64  `gorm:"not null;uniqueIndex:ux_attempts_paper_student_no,priority:2" json:"student_id"`
	Student         *User  `gorm:"foreignKey:StudentID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"student"`
	AttemptNo       int    `gorm:"not null;default:1;uniqueIndex:ux_attempts_paper_student_no,priority:3" json:"attempt_no"`
	Status          string `gorm:"type:text;not null;default:'in_progress';check:chk_attempts_status,status IN ('in_progress','submitted','grading','graded','failed')" json:"status"`
	Mode            string `gorm:"type:text;not null;default:'exam'" json:"mode"`
	Score           *float64
	FullScore       float64 `gorm:"not null;default:0" json:"full_score"`
	ObjectiveScore  *float64
	SubjectiveScore *float64
	CorrectCount    int       `gorm:"not null;default:0" json:"correct_count"`
	Report          ReportMap `gorm:"type:text;not null;default:'{}'" json:"report"`
	GradeError      string    `gorm:"type:text;not null;default:''" json:"grade_error"`
	StartedAt       time.Time `gorm:"not null" json:"started_at"`
	SubmittedAt     *time.Time
	GradedAt        *time.Time
	DurationSec     int `gorm:"not null;default:0" json:"duration_sec"`
}

func (Attempt) TableName() string { return "attempts" }

type AttemptAnswer struct {
	ID          int64      `gorm:"primaryKey;autoIncrement" json:"id"`
	AttemptID   int64      `gorm:"not null;uniqueIndex:ux_answers_attempt_question,priority:1" json:"attempt_id"`
	Attempt     *Attempt   `gorm:"foreignKey:AttemptID;constraint:OnDelete:CASCADE" json:"-"`
	QuestionID  int64      `gorm:"not null;uniqueIndex:ux_answers_attempt_question,priority:2" json:"question_id"`
	Question    *Question  `gorm:"foreignKey:QuestionID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"-"`
	Answer      StringList `gorm:"type:text;not null;default:'[]'" json:"answer"`
	IsCorrect   *bool
	Score       *float64
	MaxScore    float64    `gorm:"not null;default:0" json:"max_score"`
	GradingMode string     `gorm:"type:text;not null;default:'';check:chk_answers_mode,grading_mode IN ('','local','ai','manual')" json:"grading_mode"`
	Feedback    string     `gorm:"type:text;not null;default:''" json:"feedback"`
	Missing     StringList `gorm:"type:text;not null;default:'[]'" json:"missing"`
	Advice      string     `gorm:"type:text;not null;default:''" json:"advice"`
	UpdatedAt   time.Time  `gorm:"not null;autoUpdateTime" json:"updated_at"`
}

func (AttemptAnswer) TableName() string { return "attempt_answers" }

type Job struct {
	ID         string `gorm:"type:text;primaryKey" json:"id"`
	Type       string `gorm:"type:text;not null;check:chk_jobs_type,type IN ('paper_parse','attempt_grade','report_regen')" json:"type"`
	RefType    string `gorm:"type:text;not null;default:'';check:chk_jobs_ref,ref_type IN ('','paper','attempt')" json:"ref_type"`
	RefID      int64  `gorm:"not null;default:0" json:"ref_id"`
	Payload    string `gorm:"type:text;not null;default:'{}'" json:"-"`
	Status     string `gorm:"type:text;not null;default:'pending';check:chk_jobs_status,status IN ('pending','running','succeeded','failed','canceled')" json:"status"`
	Progress   int    `gorm:"not null;default:0" json:"progress"`
	Message    string `gorm:"type:text;not null;default:''" json:"message"`
	Attempts   int    `gorm:"not null;default:0" json:"attempts"`
	LastError  string `gorm:"type:text;not null;default:''" json:"last_error"`
	CreatedBy  *int64
	CreatedAt  time.Time `gorm:"not null;autoCreateTime" json:"created_at"`
	StartedAt  *time.Time
	FinishedAt *time.Time
}

func (Job) TableName() string { return "jobs" }

type AICall struct {
	ID               int64 `gorm:"primaryKey;autoIncrement" json:"id"`
	ProviderID       *int64
	ProviderName     string    `gorm:"type:text;not null;default:''" json:"provider_name"`
	Model            string    `gorm:"type:text;not null;default:''" json:"model"`
	Purpose          string    `gorm:"type:text;not null;default:''" json:"purpose"`
	JobID            string    `gorm:"type:text;not null;default:''" json:"job_id"`
	PromptVersion    string    `gorm:"type:text;not null;default:''" json:"prompt_version"`
	RequestDigest    string    `gorm:"type:text;not null;default:''" json:"request_digest"`
	PromptTokens     int       `gorm:"not null;default:0" json:"prompt_tokens"`
	CompletionTokens int       `gorm:"not null;default:0" json:"completion_tokens"`
	LatencyMS        int       `gorm:"not null;default:0" json:"latency_ms"`
	HTTPStatus       int       `gorm:"not null;default:0" json:"http_status"`
	Status           string    `gorm:"type:text;not null;default:'ok';check:chk_ai_calls_status,status IN ('ok','retry','failed')" json:"status"`
	Error            string    `gorm:"type:text;not null;default:''" json:"error"`
	CreatedAt        time.Time `gorm:"not null;autoCreateTime" json:"created_at"`
}

func (AICall) TableName() string { return "ai_calls" }

type Setting struct {
	Key       string    `gorm:"type:text;primaryKey" json:"key"`
	Value     string    `gorm:"type:text;not null" json:"value"`
	UpdatedAt time.Time `gorm:"not null;autoUpdateTime" json:"updated_at"`
}

func (Setting) TableName() string { return "settings" }

type AuditLog struct {
	ID        int64 `gorm:"primaryKey;autoIncrement" json:"id"`
	ActorID   *int64
	ActorName string    `gorm:"type:text;not null;default:''" json:"actor_name"`
	Action    string    `gorm:"type:text;not null" json:"action"`
	Target    string    `gorm:"type:text;not null;default:''" json:"target"`
	Detail    string    `gorm:"type:text;not null;default:'{}'" json:"detail"`
	IP        string    `gorm:"type:text;not null;default:''" json:"ip"`
	CreatedAt time.Time `gorm:"not null;autoCreateTime" json:"created_at"`
}

func (AuditLog) TableName() string { return "audit_logs" }

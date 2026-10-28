package ai

import (
	"encoding/json"
	"fmt"
)

const promptVersion = "v2"

const parseSystem = `你是一名严谨的试卷数字化助手。请把图片或文本中的试卷转写为结构化 JSON。
规则：
1. 只输出一个 JSON 对象，不要 markdown 代码块，不要任何解释文字。
2. 数学公式一律用 LaTeX：行内 $...$，独立公式 $$...$$；中文原样保留，不要翻译。
3. 题型取值：single(单选)、multiple(多选)、judge(判断)、fill(填空)、short(解答/计算/证明/简答)。
4. 每一题字段：seq(从1开始)、type、stem、options([{"key":"A","text":"..."}])、answer(字符串数组)、score、analysis、knowledge(字符串数组)、difficulty(easy|medium|hard)、confidence(0~1)、need_manual(bool)、source_index(题目所在图片在本批图片中的序号，从1开始；纯文本/PDF 为0)、bbox(本题区域)、figure(可选，图形区域)。
5. stem 从题干正文开始，不要包含题号；原图有“3.”“（3）”等题号时由系统单独显示，不要写进 stem。
6. 单选 answer 只含一个选项 key；多选含全部正确 key；判断为 ["true"] 或 ["false"]；填空按空位顺序；解答题 answer 为参考答案全文或 []。
7. 卷面没有答案时 answer 给 []，need_manual 置 true，绝对不要编造答案。
8. 分值按卷面标注；无法识别时按常见分值估计，并把 confidence 降低。
9. 跳过页眉、页脚、页码、考生信息、答题卡说明。
10. 几何图形、函数图像等不要用文字臆造或省略；题干保留“如图”等原文。
11. bbox 是本题在图片中的矩形，格式 {"x":0.0,"y":0.0,"w":0.0,"h":0.0}，坐标为相对整张图片的 0~1 比例；区域从题干正文开始（不含题号），到本题结束。图形题另给 figure，格式相同，只框图形本身，不含题号和文字；没有图形时省略 figure。figure 必须完整包含图形的全部线条与标注，宁可略大留少量白边，绝对不能切掉图形边缘；bbox 坐标宁可略小，不要框进上一题或下一题。
JSON 结构：{"title":"","stage":"junior|senior","grade":"","subject":"","duration_min":0,"total_score":0,"questions":[...]}`

const gradeSystem = `你是初中/高中数学阅卷老师，请根据题目、参考答案、解析和学生答案评分。
规则：
1. 只输出 JSON 数组，不要 markdown 代码块，不要解释。
2. 每项字段：{"question_id":整数,"score":数字,"is_correct":布尔,"feedback":"≤80字","missing":["遗漏点"],"advice":"改进建议"}。
3. 按步骤给分；等价表达（\frac{1}{2} 与 0.5）不扣分；学生答案含 LaTeX 时按数学含义理解；不得因排版、写法差异扣分。
4. score 必须在 0 到该题满分之间，且为 0.5 的倍数；is_correct 仅当满分时为 true。
5. 学生答案为空时给 0 分，feedback 说明未作答。
6. 只对给出的题目评分，不要新增题目。`

const equivSystem = `你是数学答案等价性判定器。判断学生答案与参考答案在数学上是否等价。
只输出 JSON：{"items":[{"question_id":整数,"equivalent":布尔,"reason":"≤40字"}]}
忽略排版、空格、等价变形；数值不同则不等价；空答案视为不等价。`

const reportSystem = `你是学习规划老师。根据整卷统计与逐题结果，输出学习报告。
只输出 JSON：{"summary":"120字以内的总体评价","suggestions":["3~5条具体建议"],"weak_points":[{"knowledge":"知识点","wrong":0,"total":0}]}
要求：评价具体、可执行，不空泛；建议针对错题知识点；不要输出 Markdown。`

func TextPart(text string) ContentPart {
	return ContentPart{Type: "text", Text: text}
}

func ImagePart(dataURL string) ContentPart {
	return ContentPart{Type: "image_url", ImageURL: &ImageURL{URL: dataURL}}
}

func ParseMessages(hint string, pages int, text string) []Message {
	user := fmt.Sprintf("请识别以下试卷内容（共 %d 页/批）。", pages)
	if hint != "" {
		user += "\n卷面提示：" + hint
	}
	if text != "" {
		user += "\n试卷文本：\n" + text
	}
	return []Message{
		{Role: "system", Content: parseSystem},
		{Role: "user", Content: user},
	}
}

func GradeMessages(payload any) ([]Message, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return []Message{
		{Role: "system", Content: gradeSystem},
		{Role: "user", Content: string(b)},
	}, nil
}

func EquivMessages(payload any) ([]Message, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return []Message{
		{Role: "system", Content: equivSystem},
		{Role: "user", Content: string(b)},
	}, nil
}

func ReportMessages(payload any) ([]Message, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return []Message{
		{Role: "system", Content: reportSystem},
		{Role: "user", Content: string(b)},
	}, nil
}

type ParsedQuestion struct {
	Seq         int          `json:"seq"`
	Type        string       `json:"type"`
	Stem        string       `json:"stem"`
	Options     []OptionJSON `json:"options"`
	Answer      []string     `json:"answer"`
	Score       float64      `json:"score"`
	Analysis    string       `json:"analysis"`
	Knowledge   []string     `json:"knowledge"`
	Difficulty  string       `json:"difficulty"`
	Confidence  float64      `json:"confidence"`
	NeedManual  bool         `json:"need_manual"`
	SourceIndex int          `json:"source_index"`
	SourceFile  string       `json:"-"`
	BBox        *ImageBox    `json:"bbox"`
	Figure      *ImageBox    `json:"figure"`
}

type ImageBox struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	W float64 `json:"w"`
	H float64 `json:"h"`
}

type OptionJSON struct {
	Key  string `json:"key"`
	Text string `json:"text"`
}

type ParsedPaper struct {
	Title       string           `json:"title"`
	Stage       string           `json:"stage"`
	Grade       string           `json:"grade"`
	Subject     string           `json:"subject"`
	DurationMin int              `json:"duration_min"`
	TotalScore  float64          `json:"total_score"`
	Questions   []ParsedQuestion `json:"questions"`
}

type GradeItem struct {
	QuestionID int64    `json:"question_id"`
	Score      float64  `json:"score"`
	IsCorrect  bool     `json:"is_correct"`
	Feedback   string   `json:"feedback"`
	Missing    []string `json:"missing"`
	Advice     string   `json:"advice"`
}

type EquivItem struct {
	QuestionID int64  `json:"question_id"`
	Equivalent bool   `json:"equivalent"`
	Reason     string `json:"reason"`
}

type EquivResult struct {
	Items []EquivItem `json:"items"`
}

type ReportResult struct {
	Summary     string            `json:"summary"`
	Suggestions []string          `json:"suggestions"`
	WeakPoints  []ReportWeakPoint `json:"weak_points"`
}

type ReportWeakPoint struct {
	Knowledge string `json:"knowledge"`
	Wrong     int    `json:"wrong"`
	Total     int    `json:"total"`
}

const answerSystem = `你是学科命题老师。请根据题干、选项和学科知识，为缺少参考答案的题目补全答案。
规则：
1. 只输出一个 JSON 对象，不要 markdown 代码块，不要解释。
2. 必须依据题干与选项作答；信息不足或无法确定时 answer 返回 []，绝对不要编造。
3. 单选 answer 为一个选项 key；多选为全部正确 key；判断为 ["true"] 或 ["false"]；填空按空位顺序；解答给出简洁参考答案。
4. 可选返回 analysis（不超过 100 字）与 confidence（0~1）。
JSON 结构：{"items":[{"question_id":1,"answer":["A"],"analysis":"","confidence":0.9}]}`

type AnswerItem struct {
	QuestionID int64    `json:"question_id"`
	Answer     []string `json:"answer"`
	Analysis   string   `json:"analysis"`
	Confidence float64  `json:"confidence"`
}

type AnswerResult struct {
	Items []AnswerItem `json:"items"`
}

func AnswerMessages(payload any) ([]Message, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return []Message{
		{Role: "system", Content: answerSystem},
		{Role: "user", Content: string(b)},
	}, nil
}

const reviewSystem = `你是学科老师。请独立解答题目，并核对给出的参考答案是否正确；答案缺失或错误时给出正确答案，答案正确时保持原答案。
规则：
1. 只输出一个 JSON 对象，不要 markdown 代码块，不要解释。
2. answer 必须符合题型：单选一个选项 key；多选全部 key；判断 ["true"]/["false"]；填空按空位顺序；解答给参考答案。
3. is_correct 表示原参考答案是否数学上正确/等价；原答案为 [] 时为 false。
4. feedback 说明复核结论或修改原因（≤80 字）；analysis 可补充解析（≤100 字）；confidence 0~1。
JSON 结构：{"answer":["A"],"is_correct":true,"feedback":"","analysis":"","confidence":0.95}`

type ReviewResult struct {
	Answer     []string `json:"answer"`
	IsCorrect  bool     `json:"is_correct"`
	Feedback   string   `json:"feedback"`
	Analysis   string   `json:"analysis"`
	Confidence float64  `json:"confidence"`
}

func ReviewMessages(payload any) ([]Message, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return []Message{
		{Role: "system", Content: reviewSystem},
		{Role: "user", Content: string(b)},
	}, nil
}

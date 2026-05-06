package skill

import "strings"

type TimeRange struct {
	StartMs    int64
	EndMs      int64
	StartDt    string
	EndDt      string
	Label      string
	RollupUnit string
}

type Skill struct {
	Name        string
	Description string
	Workflows   []string // workflow segment names
	ToolGroups  []string // additional tool prompt groups
	SkipCore    bool     // if true, skip Core (sql_tools etc)
	Guards      []string // guard names to activate
	Hint        string   // hint to add to user message
	AllowTools  []string // if set, only these tools are exposed to LLM (nil = all tools)
}

type Registry struct {
	skills       map[string]*Skill
	defaultSkill *Skill
}

func NewRegistry() *Registry {
	r := &Registry{skills: make(map[string]*Skill)}
	r.Register(BasicAnalysisSkill())
	r.Register(AdvancedAnalysisSkill())
	r.Register(ReportSkill())
	r.Register(DocLookupSkill())
	r.Register(DataQuerySkill())
	r.Register(TimerSkill())
	r.Register(GeneralSkill())
	r.defaultSkill = r.skills["General"]
	return r
}

func (r *Registry) Register(s *Skill) {
	r.skills[s.Name] = s
}

func (r *Registry) Get(name string) *Skill {
	return r.skills[name]
}

// containsKeyword returns true if s contains any of the keywords.
func containsKeyword(s string, keywords []string) bool {
	for _, kw := range keywords {
		if strings.Contains(s, kw) {
			return true
		}
	}
	return false
}

// Classify determines the appropriate skill for a query based on keywords.
func (r *Registry) Classify(query string) *Skill {
	lower := strings.ToLower(query)

	// 1. DocLookup: 질문/개념/사용법 패턴이 있으면 최우선 (실행 작업이 아닌 지식 질문)
	questionPatterns := []string{
		"뭐야", "뭔가요", "란?", "이란", "사용법", "문법", "예제", "알려줘", "설명해", "어떻게",
		"how to", "what is", "what are", "explain", "usage", "example", "syntax", "help me understand",
	}
	docContextKeywords := []string{"문서", "매뉴얼", "manual", "doc", "documentation", "reference"}
	if containsKeyword(lower, questionPatterns) || containsKeyword(lower, docContextKeywords) {
		return r.skills["DocLookup"]
	}

	// 2. Report (most specific action)
	reportKeywords := []string{"리포트", "보고서", "report", "summary report"}
	if containsKeyword(lower, reportKeywords) {
		return r.skills["Report"]
	}

	// 3. Timer (스케줄러/타이머 생성·관리)
	timerKeywords := []string{
		"타이머", "스케줄", "스케줄러", "주기적", "반복 실행", "수집 설정", "수집",
		"timer", "scheduler", "schedule", "cron", "every", "periodic", "interval", "collect",
	}
	if containsKeyword(lower, timerKeywords) {
		return r.skills["Timer"]
	}

	// 4. Advanced analysis (고급 분석)
	advancedKeywords := []string{
		"심층", "다각도", "고급", "fft", "rms", "스펙트럼", "엔벨로프",
		"진동 분석", "이상치", "이상 탐지",
		"advanced", "spectrum", "envelope", "anomaly", "vibration analysis",
		"frequency", "crest factor", "peak-to-peak",
	}
	if containsKeyword(lower, advancedKeywords) {
		return r.skills["AdvancedAnalysis"]
	}

	// 5. Basic analysis (시각화 의도가 있는 요청)
	analysisKeywords := []string{
		"분석", "대시보드", "차트", "시각화", "추세", "트렌드", "패턴", "비교", "보여줘", "보여 줘", "그래프",
		"dashboard", "chart", "visualize", "visualization", "trend", "pattern", "compare", "comparison",
		"show me", "plot", "graph", "analyze", "analysis", "display",
	}
	if containsKeyword(lower, analysisKeywords) {
		return r.skills["BasicAnalysis"]
	}

	// 6. DataQuery: 순수 데이터 조회 (시각화 없이 결과만)
	dataQueryKeywords := []string{
		"조회", "확인", "최근", "최신", "태그", "몇건", "몇 건",
		"query", "fetch", "retrieve", "select", "count", "how many",
		"latest", "recent", "list", "get data", "check",
	}
	if containsKeyword(lower, dataQueryKeywords) {
		return r.skills["DataQuery"]
	}

	// 7. "데이터"/"data" alone → DataQuery
	if strings.Contains(lower, "데이터") || strings.Contains(lower, "data") {
		return r.skills["DataQuery"]
	}

	return r.defaultSkill
}

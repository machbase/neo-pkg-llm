package context

var ToolPrompts = map[string]string{
	"sql_tools": `## SQL 도구 사용법
- execute_sql_query: 직접 SQL 실행. GROUP BY 없이 사용 가능.
- timeformat: "ms" 파라미터로 지정! SQL 안에 넣지 마세요!
- UPDATE 구문 사용 금지
- 통계 조회: SELECT NAME, COUNT(*), AVG(VALUE) FROM 테이블 GROUP BY NAME
- 시간 범위 확인: SELECT MIN(TIME), MAX(TIME) FROM 테이블 (timeformat:"ms")
`,

	"tql_tools": `## TQL 도구 사용법
- TQL의 SQL() 안에서는 반드시 GROUP BY 포함!
- TQL SQL()에서 큰따옴표(") 사용 금지 → 백틱 사용!
- TQL SQL()에서 ROLLUP alias 사용 금지! 표현식 직접 사용
- TQL에서 SQL()은 파일당 1회만 사용 가능
- save_tql_file: 파일명/폴더명은 반드시 영어로만! 한글 절대 금지!
- TEMPLATE 형식: TEMPLATE:ID TABLE:테이블명 TAG:태그명 UNIT:단위
- UNIT 선택: 수시간→'sec', 수일→'hour', 수주~수년→'day'
`,

	"dashboard_tools": `## 대시보드 도구 사용법
- create_dashboard_with_charts: 반드시 이 도구 하나로! (create_dashboard + add_chart 조합 금지!)
- filename: "테이블명/테이블명_Dashboard.dsh" 형식 (영어로만!)
- title: 의미 있는 영어 이름! (예: "GOLD Analysis Dashboard")
- time_start, time_end: 에폭 밀리초 숫자를 문자열로 전달! "auto", "now-1d" 등 금지!
- charts: {title, type, tql_path} 또는 {title, type, table, tag, column}
- chart title: 각 차트의 내용을 설명하는 이름!
`,

	"doc_tools": `## 문서 도구 사용법
- 카탈로그에서 키워드로 검색 → 해당 행의 경로를 그대로 복사하여 사용
- 파일명을 추측해서 만들기 금지!
- 카탈로그에서 찾은 경로를 get_full_document_content에 전달
- 실행 작업(B유형)에서 문서 도구를 연달아 호출 금지 (1회 참조 후 반드시 실행 도구 호출)
- 문서 링크 및 문서 탐색 제안 금지

## 답변 작성 규칙
- 문서 원문을 그대로 복사하지 마세요! 사용자 질문에 맞게 **핵심만 요약**하세요.
- 코드/SQL 예제는 사용자 질문과 직접 관련된 것만 1~2개 포함하세요.
- 표(table)는 핵심 행만 발췌하고, 전체를 그대로 붙여넣지 마세요.
- "자세한 내용은 문서를 참고하세요" 같은 안내 금지 (문서 링크 제공 금지)
`,

	"report_tools": `## 리포트 도구 사용법
- save_html_report: 통계/태그/시간범위 조회를 직접 하지 마세요. 이 도구가 내부에서 모두 처리.
- table만 지정하여 바로 호출. 도구 호출 시 모든 파라미터를 빠짐없이 전달.
- 템플릿 ID: 운전/차량='R-3', 진동='R-2', 금융='R-1', 범용='R-0'
`,

	"common_prohibitions": `## 금지사항
- 도구 호출 없이 답변 절대 금지! 어떤 질문이든 최소 1개 도구를 호출한 후 답변하세요.
- 문서 경로를 추측하거나 만들지 마세요! 반드시 카탈로그에서 찾은 경로를 그대로 복사해서 사용하세요.
- 빈 객체({})를 값으로 넣지 마세요. 생략하거나 정확한 값을 넣으세요.
- 기본 접속 정보: host=127.0.0.1, port=5654 (자동 적용됨, 별도 지정 불필요)
`,
}

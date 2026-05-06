package skill

// DataQuerySkill handles pure data retrieval requests:
// "데이터 조회해줘", "태그 확인", "테이블 뭐 있어", "몇건이야", "최근 데이터" 등
// 대시보드/시각화 없이 SQL 결과만 반환.
func DataQuerySkill() *Skill {
	return &Skill{
		Name:        "DataQuery",
		Description: "시계열 데이터 조회/확인 (결과만 반환, 시각화 없음)",
		Workflows:   []string{},
		ToolGroups:  []string{},
		Guards:      []string{},
		Hint:        "사용자가 요청한 데이터를 SQL로 조회하여 결과를 텍스트로 알려주세요. 대시보드나 차트를 만들지 마세요.",
		AllowTools: []string{
			"list_tables", "list_table_tags", "execute_sql_query",
		},
	}
}

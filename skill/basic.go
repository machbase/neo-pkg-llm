package skill

func BasicAnalysisSkill() *Skill {
	return &Skill{
		Name:        "BasicAnalysis",
		Description: "기본 분석 (table-based 차트 대시보드)",
		Workflows:   []string{"BasicWorkflow"},
		ToolGroups:  []string{"dashboard_tools"},
		Guards:      []string{},
		Hint:        "기본 분석(table-based 차트) 절차를 따르세요.",
		AllowTools: []string{
			"list_tables", "list_table_tags", "execute_sql_query",
			"create_dashboard_with_charts", "preview_dashboard",
			"list_available_documents", "get_full_document_content",
		},
	}
}

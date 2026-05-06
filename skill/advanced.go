package skill

func AdvancedAnalysisSkill() *Skill {
	return &Skill{
		Name:        "AdvancedAnalysis",
		Description: "고급 분석 (TQL 템플릿 기반 심층 분석)",
		Workflows:   []string{"AdvancedWorkflow"},
		ToolGroups:  []string{"tql_tools", "dashboard_tools"},
		Guards:      []string{"dashboard_early", "chart_omission"},
		Hint:        "고급 분석(TQL 템플릿) 절차를 따르세요.",
		AllowTools: []string{
			"list_tables", "list_table_tags", "execute_sql_query",
			"create_folder", "save_tql_file", "validate_chart_tql",
			"create_dashboard_with_charts", "preview_dashboard",
			"list_available_documents", "get_full_document_content",
		},
	}
}

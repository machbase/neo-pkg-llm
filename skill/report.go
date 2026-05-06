package skill

func ReportSkill() *Skill {
	return &Skill{
		Name:        "Report",
		Description: "HTML 분석 리포트 생성",
		Workflows:   []string{"HTMLReportWorkflow"},
		ToolGroups:  []string{"report_tools"},
		Guards:      []string{"report_omission"},
		Hint:        "save_html_report를 바로 호출하세요.",
		AllowTools: []string{
			"list_tables", "list_table_tags", "execute_sql_query",
			"save_html_report",
		},
	}
}

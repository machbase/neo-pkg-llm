package skill

// TimerSkill handles timer/scheduler creation and management:
// "타이머 만들어줘", "스케줄러 설정", "10초마다 수집", "create a timer" 등
func TimerSkill() *Skill {
	return &Skill{
		Name:        "Timer",
		Description: "타이머/스케줄러 생성·관리",
		Workflows:   []string{},
		ToolGroups:  []string{},
		Guards:      []string{},
		Hint: "반드시 다음 절차를 따르세요:\n" +
			"1) get_full_document_content로 'utilities/timer-templates.md' 문서를 먼저 조회\n" +
			"2) execute_sql_query로 TAG TABLE 생성\n" +
			"3) save_tql_file로 TQL 스크립트 저장\n" +
			"4) add_timer로 타이머 생성\n" +
			"5) start_timer로 시작",
		AllowTools: []string{
			"list_timers", "add_timer", "start_timer", "stop_timer", "delete_timer",
			"execute_sql_query", "save_tql_file", "create_folder", "delete_file",
			"list_available_documents", "get_full_document_content",
		},
	}
}

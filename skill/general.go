package skill

func GeneralSkill() *Skill {
	return &Skill{
		Name:        "General",
		Description: "일반 대화/인사/분류 불가 쿼리",
		Workflows:   []string{"QueryClassification"},
		ToolGroups:  []string{},
		SkipCore:    true,
		Guards:      []string{},
		Hint:        "",
	}
}

package skill

func DocLookupSkill() *Skill {
	return &Skill{
		Name:        "DocLookup",
		Description: "문서 검색/조회",
		Workflows:   []string{"QueryClassification"},
		ToolGroups:  []string{"doc_tools"},
		SkipCore:    true,
		Guards:      []string{},
		Hint:        "",
		AllowTools: []string{
			"list_available_documents", "get_full_document_content",
			"get_document_sections", "extract_code_blocks",
		},
	}
}

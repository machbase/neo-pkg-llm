package fixer

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// fixTQLContent handles template expansion and line break fixing for TQL content.
func fixTQLContent(name string, args map[string]any, fctx *FixerContext) {
	tql, ok := args["tql_content"].(string)
	if !ok {
		// If no tql_content but description has TEMPLATE ref, use description as source
		if desc, ok := args["description"].(string); ok {
			if TemplateRefRE.MatchString(desc) {
				args["tql_content"] = desc
				tql = desc
			} else {
				return
			}
		} else {
			return
		}
	}
	tql = strings.TrimSpace(tql)

	// Also check description field for TEMPLATE ref when tql_content is raw TQL
	refSource := tql
	if desc, ok := args["description"].(string); ok && TemplateRefRE.MatchString(desc) {
		refSource = desc
	}

	match := TemplateRefRE.FindStringSubmatch(refSource)
	if match != nil {
		params := map[string]string{}
		if m := templateTableRE.FindStringSubmatch(refSource); m != nil {
			params["TABLE"] = m[1]
		}
		if m := templateUnitRE.FindStringSubmatch(refSource); m != nil {
			params["UNIT"] = m[1]
		}
		if m := templateTagRE.FindStringSubmatch(refSource); m != nil {
			params["TAG"] = m[1]
		}
		if m := templateTag1RE.FindStringSubmatch(refSource); m != nil {
			params["TAG1"] = m[1]
		}
		if m := templateTag2RE.FindStringSubmatch(refSource); m != nil {
			params["TAG2"] = m[1]
		}
		// Inject time range
		if fctx.TimeStartDt != "" {
			params["TIME_START"] = fctx.TimeStartDt
			params["TIME_END"] = fctx.TimeEndDt
		} else if fctx.DataMinDt != "" && fctx.DataMaxDt != "" {
			params["TIME_START"] = fctx.DataMinDt
			params["TIME_END"] = fctx.DataMaxDt
		} else {
			params["TIME_START"] = "1970-01-01 00:00:00"
			params["TIME_END"] = time.Now().Format(DtFormat)
		}
		if ExpandTemplateFunc != nil {
			expanded, err := ExpandTemplateFunc(match[1], params)
			if err == nil {
				args["tql_content"] = expanded
				fmt.Printf("  [fix] Template %s expanded\n", match[1])
			}
		}
	} else {
		// Try to auto-detect template from filename
		autoExpanded := false
		if fn, ok := args["filename"].(string); ok {
			if idMatch := TemplateIDRE.FindString(fn); idMatch != "" {
				idMatch = strings.ReplaceAll(idMatch, "_", "-")
				table := strings.Split(fn, "/")[0]
				nameRE := regexp.MustCompile(`NAME\s*=\s*'([^']+)'`)
				unitRE := regexp.MustCompile(`ROLLUP\('(\w+)'`)
				tag := ""
				unit := "'day'"
				if m := nameRE.FindStringSubmatch(tql); m != nil {
					tag = m[1]
				}
				if m := unitRE.FindStringSubmatch(tql); m != nil {
					unit = "'" + m[1] + "'"
				}
				params := map[string]string{"TABLE": table, "UNIT": unit}
				if idMatch == "1-4" || idMatch == "3-2" {
					tagsRE := regexp.MustCompile(`'([^']+)'`)
					allTags := tagsRE.FindAllStringSubmatch(tql, -1)
					var names []string
					skipUnits := map[string]bool{"day": true, "hour": true, "sec": true, "min": true, "week": true, "month": true}
					for _, t := range allTags {
						if !skipUnits[t[1]] {
							names = append(names, t[1])
						}
					}
					if len(names) >= 2 {
						params["TAG1"] = names[0]
						params["TAG2"] = names[1]
					}
				} else if tag != "" {
					params["TAG"] = tag
				}
				// Extract TO_DATE from raw TQL
				toDateExtractRE := regexp.MustCompile(`TO_DATE\('([^']+)'\)`)
				dates := toDateExtractRE.FindAllStringSubmatch(tql, 2)
				if len(dates) >= 2 {
					params["TIME_START"] = dates[0][1]
					params["TIME_END"] = dates[1][1]
				} else if fctx.TimeStartDt != "" {
					params["TIME_START"] = fctx.TimeStartDt
					params["TIME_END"] = fctx.TimeEndDt
				} else if fctx.DataMinDt != "" && fctx.DataMaxDt != "" {
					params["TIME_START"] = fctx.DataMinDt
					params["TIME_END"] = fctx.DataMaxDt
				} else {
					fmt.Printf("  [skip] Raw TQL auto-expand skipped (no time range)\n")
					return
				}
				if ExpandTemplateFunc != nil {
					expanded, err := ExpandTemplateFunc(idMatch, params)
					if err == nil {
						args["tql_content"] = expanded
						autoExpanded = true
						fmt.Printf("  [fix] Raw TQL → template %s auto-expanded\n", idMatch)
					}
				}
			}
		}
		if !autoExpanded {
			// Fix TQL line breaks
			args["tql_content"] = TqlFuncRE.ReplaceAllStringFunc(tql, func(s string) string {
				idx := strings.Index(s, ")")
				return s[:idx+1] + "\n" + strings.TrimSpace(s[idx+1:])
			})
		}
	}
}

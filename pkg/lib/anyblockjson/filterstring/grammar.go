package filterstring

// EBNF is the grammar this parser pins (SPEC §6.2.1 — the parser is the
// normative artifact; this text is what the API discovery surface serves,
// and the Phase-5 GBNF conversion consumes it). Keywords are matched
// case-insensitively; canonical rendering is uppercase.
const EBNF = `filter      = orExpr ;
orExpr      = andExpr , { "OR" , andExpr } ;
andExpr     = primary , { "AND" , primary } ;
primary     = "(" , orExpr , ")" | leaf ;
leaf        = key , condition ;
condition   = compare , value
            | ( "=" | "!=" ) , valueList            (* set literal: exactIn / notExactIn *)
            | [ "NOT" ] , "CONTAINS" , value
            | [ "NOT" ] , "IN" , valueList
            | [ "NOT" ] , "HAS" , "ALL" , valueList
            | "IS" , [ "NOT" ] , "EMPTY"
            | "EXISTS" ;
compare     = "=" | "!=" | ">" | "<" | ">=" | "<=" ;
valueList   = "(" , value , { "," , value } , ")" ;
value       = string | number | "true" | "false" | preset ;
preset      = presetName , "(" , ")" | countingName , "(" , number , ")" ;
presetName  = "yesterday" | "today" | "tomorrow" | "lastWeek" | "currentWeek"
            | "nextWeek" | "lastMonth" | "currentMonth" | "nextMonth"
            | "lastYear" | "currentYear" | "nextYear" ;
countingName = "daysAgo" | "daysFromNow" ;
key         = identifier ;                          (* a bare property key *)
string      = '"' , { character } , '"' ;           (* backslash escapes: \" \\ \n \t *)
`

// Examples are worked filter strings, served alongside the grammar (C12).
var Examples = []string{
	`done = false AND (dueDate < currentWeek() OR dueDate IS EMPTY)`,
	`status IN ("In progress", "Blocked")`,
	`tags HAS ALL ("urgent", "q3") AND assignee IS NOT EMPTY`,
	`lastModifiedDate > daysAgo(7)`,
	`type IN ("task", "bug") AND priority >= 3`,
	`name CONTAINS "report" AND dueDate < "2026-08-01"`,
}

package anymark

import (
	"regexp"
	"strings"
)

var (
	styles = regexp.MustCompile(`<style[\s\S]*?>([\s\S]*?)</style>`)
	class  = regexp.MustCompile(`span.[\s\S]*? `)
)

const underscoreStyle = "text-decoration: underline"

// transformCSSUnderscore replace string with css style "text-decoration: underline" with <u></u> tag
func transformCSSUnderscore(source string) string {
	style := styles.FindStringSubmatch(source)
	if len(style) == 0 {
		return source
	}
	allStyles := strings.Split(style[0], "\n")
	underscoreCSS := make([]string, 0)
	for _, style := range allStyles {
		//span.s1 {"text-decoration: underline}
		if strings.Contains(style, underscoreStyle) {
			cssClass := class.FindStringSubmatch(style)
			if len(cssClass) != 0 {
				//span.s1
				className := strings.Split(cssClass[0], ".")
				if len(className) >= 2 {
					underscoreCSS = append(underscoreCSS, strings.TrimSpace(className[1]))
				}

			}
		}

	}
	for _, class := range underscoreCSS {
		underscore := regexp.MustCompile(`<span class=\"` + class + `\"[\s\S]*?>([\s\S]*?)</span>`)
		source = underscore.ReplaceAllString(source, "<u>"+`$1`+"</u>")

	}
	return source
}

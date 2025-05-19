package objectcreator

import (
	"strings"

	"github.com/gosimple/unidecode"
	"github.com/iancoleman/strcase"
)

func transliterate(in string) string {
	out := unidecode.Unidecode(strings.TrimSpace(in))
	return strcase.ToSnake(out)
}

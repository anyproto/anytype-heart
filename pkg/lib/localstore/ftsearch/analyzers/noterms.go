package analyzers

import (
	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/analysis/analyzer/custom"
	"github.com/blevesearch/bleve/v2/analysis/char/regexp"
	"github.com/blevesearch/bleve/v2/analysis/token/lowercase"
	"github.com/blevesearch/bleve/v2/analysis/tokenizer/single"
	"github.com/blevesearch/bleve/v2/mapping"
)

const noTermsName = "noTerms"

func AddNoTermsAnalyzer(indexMapping *mapping.IndexMappingImpl) error {
	err := addCharFilter(indexMapping)
	if err != nil {
		return err
	}
	return addAnalyzer(indexMapping)
}

func GetNoTermsFieldMapping() *mapping.FieldMapping {
	keywordMapping := bleve.NewTextFieldMapping()
	keywordMapping.Analyzer = noTermsName
	return keywordMapping
}

func addAnalyzer(indexMapping *mapping.IndexMappingImpl) error {
	return indexMapping.AddCustomAnalyzer(noTermsName,
		map[string]interface{}{
			"type":      custom.Name,
			"tokenizer": single.Name,
			"token_filters": []string{
				lowercase.Name,
			},
			"char_filters": []string{
				regexp.Name,
			},
		})
}

func addCharFilter(indexMapping *mapping.IndexMappingImpl) error {
	return indexMapping.AddCustomCharFilter(regexp.Name, map[string]interface{}{
		"regexp":  "[\\n\\t\\r]+",
		"replace": " ",
		"type":    regexp.Name,
	})
}

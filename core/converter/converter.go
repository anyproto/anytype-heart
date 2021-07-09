package converter

type Converter interface {
	Convert() (result []byte)
	SetKnownLinks(ids []string) Converter
	FileHashes() []string
	ImageHashes() []string
	Ext() string
}

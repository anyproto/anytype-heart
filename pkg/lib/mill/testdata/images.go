package testdata

type TestImage struct {
	Path    string
	Format  string
	HasExif bool
	Width   int
	Height  int
}

var Images = []TestImage{
	{
		Path:    "testdata/image.jpeg",
		Format:  "jpeg",
		HasExif: true,
		Width:   1024,
		Height:  786,
	},
	{
		Path:    "testdata/image.png",
		Format:  "png",
		HasExif: false,
		Width:   300,
		Height:  300,
	},
	{
		Path:    "testdata/image.gif",
		Format:  "gif",
		HasExif: false,
		Width:   300,
		Height:  187,
	},
}

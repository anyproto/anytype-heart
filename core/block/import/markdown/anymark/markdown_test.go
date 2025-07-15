package anymark

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
)

type MdCase struct {
	MD     string                   `json:"md"`
	Blocks []map[string]interface{} `json:"blocks"`
	Desc   string                   `json:"desc"`
}

func TestConvertMdToBlocks(t *testing.T) {
	t.Run("markdown to blocks", func(t *testing.T) {
		bs, err := os.ReadFile("testdata/md_cases.json")
		if err != nil {
			panic(err)
		}
		var testCases []MdCase
		if err := json.Unmarshal(bs, &testCases); err != nil {
			panic(err)
		}

		for testNum, testCase := range testCases {
			t.Run(testCase.Desc, func(t *testing.T) {
				blocks, _, err := MarkdownToBlocks([]byte(testCases[testNum].MD), "", []string{})
				require.NoError(t, err)
				replaceFakeIds(blocks)

				actualJson, err := json.Marshal(blocks)
				require.NoError(t, err)

				var actual []map[string]interface{}
				err = json.Unmarshal(actualJson, &actual)
				require.NoError(t, err)

				if !reflect.DeepEqual(testCase.Blocks, actual) {
					fmt.Println("expected:\n", string(actualJson))
					require.Equal(t, testCase.Blocks, actual)
				}
			})
		}
	})

	t.Run("markdown links with special characters", func(t *testing.T) {
		testCases := []struct {
			name         string
			markdown     string
			basePath     string
			expectedLink string
			isImage      bool
		}{
			{
				name:         "relative path with emoji",
				markdown:     "[link](📁/readme.md)",
				basePath:     "/test",
				expectedLink: "/test/📁/readme.md",
			},
			{
				name:         "relative path with spaces percent encoded",
				markdown:     "[link](my%20docs/readme.md)",
				basePath:     "/test",
				expectedLink: "/test/my docs/readme.md",
			},
			{
				name:         "url with emoji",
				markdown:     "[link](https://example.com/📁/page)",
				basePath:     "/test",
				expectedLink: "https://example.com/📁/page",
			},
			{
				name:         "file with no extension",
				markdown:     "[link](README)",
				basePath:     "/test",
				expectedLink: "/test/README.md",
			},
			{
				name:         "file with extension",
				markdown:     "[link](doc.pdf)",
				basePath:     "/test",
				expectedLink: "/test/doc.pdf",
			},
			{
				name:         "ftp url",
				markdown:     "[link](ftp://files.example.com/doc.pdf)",
				basePath:     "/test",
				expectedLink: "ftp://files.example.com/doc.pdf",
			},
			{
				name:         "mailto link",
				markdown:     "[link](mailto:test@example.com)",
				basePath:     "/test",
				expectedLink: "mailto:test@example.com",
			},
			{
				name:         "data url",
				markdown:     "[link](data:text/plain;base64,SGVsbG8=)",
				basePath:     "/test",
				expectedLink: "data:text/plain;base64,SGVsbG8=",
			},
			{
				name:         "windows path treated as scheme",
				markdown:     "[link](C:\\Users\\docs\\readme.md)",
				basePath:     "/test",
				expectedLink: "C:\\Users\\docs\\readme.md",
			},
			{
				name:         "path with hash",
				markdown:     "[link](docs/readme.md#section)",
				basePath:     "/test",
				expectedLink: "/test/docs/readme.md",
			},
			{
				name:         "relative path link",
				markdown:     "Link to [readme](../docs/readme.md)",
				basePath:     "/Users/test/project",
				expectedLink: "/Users/test/docs/readme.md",
			},
			{
				name:         "relative path with invalid percent",
				markdown:     "[link](docs/100%/readme.md)",
				basePath:     "/test",
				expectedLink: "/test/docs/100%/readme.md",
			},
			{
				name:         "anytype link",
				markdown:     "Link to [obj](anytype://123)",
				basePath:     "/Users/test/project",
				expectedLink: "anytype://123",
			},
			{
				name:         "anytype link with invalid escape",
				markdown:     "Link to [obj](anytype://123/%zz)",
				basePath:     "/Users/test/project",
				expectedLink: "anytype://123/%zz",
			},
			{
				name:         "anytype image",
				markdown:     "![img](anytype://image)",
				basePath:     "/Users/test/project",
				expectedLink: "anytype://image",
				isImage:      true,
			},
			{
				name:         "relative path image",
				markdown:     "![img](../images/screenshot.png)",
				basePath:     "/Users/test/project",
				expectedLink: "/Users/test/images/screenshot.png",
				isImage:      true,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				blocks, _, err := MarkdownToBlocks([]byte(tc.markdown), tc.basePath, nil)
				require.NoError(t, err)

				if tc.isImage {
					var imgName string
					for _, b := range blocks {
						if f := b.GetFile(); f != nil {
							imgName = f.GetName()
							break
						}
					}
					require.Equal(t, tc.expectedLink, imgName)
				} else {
					var markParam string
					for _, b := range blocks {
						if b.GetText() != nil && b.GetText().GetMarks() != nil {
							marks := b.GetText().GetMarks().GetMarks()
							if len(marks) > 0 {
								markParam = marks[0].GetParam()
								break
							}
						}
					}
					require.Equal(t, tc.expectedLink, markParam)
				}
			})
		}
	})
}

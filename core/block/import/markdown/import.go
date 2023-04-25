package markdown

import (
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/globalsign/mgo/bson"
	"github.com/gogo/protobuf/types"
	"github.com/google/uuid"
	"github.com/pkg/errors"

	"github.com/anytypeio/go-anytype-middleware/core/block/import/converter"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/anytypeio/go-anytype-middleware/util/slice"
)

var (
	emojiAproxRegexp = regexp.MustCompile(`[\x{2194}-\x{329F}\x{1F000}-\x{1FADF}]`)

	log          = logging.Logger("markdown-import")
	articleIcons = []string{"📓", "📕", "📗", "📘", "📙", "📖", "📔", "📒", "📝", "📄", "📑"}
)

func init() {
	converter.RegisterFunc(New)
}

type Markdown struct {
	blockConverter *mdConverter
}

const Name = "Notion"

func New(tempDirProvider core.TempDirProvider) converter.Converter {
	return &Markdown{blockConverter: newMDConverter(tempDirProvider)}
}

func (m *Markdown) Name() string {
	return Name
}

func (m *Markdown) GetParams(params pb.IsRpcObjectImportRequestParams) (string, error) {
	if p, ok := params.(*pb.RpcObjectImportRequestParamsOfNotionParams); ok {
		return p.NotionParams.GetPath(), nil
	}
	return "", errors.Wrap(errors.New("wrong parameters format"), "Markdown: GetParams")
}

func (m *Markdown) GetImage() ([]byte, int64, int64, error) {
	return nil, 0, 0, nil
}

func (m *Markdown) GetSnapshots(req *pb.RpcObjectImportRequest) *converter.Response {
	path, err := m.GetParams(req.Params)
	allErrors := converter.NewError()
	if err != nil {
		allErrors.Add(path, err)
		return &converter.Response{Error: allErrors}
	}
	files, allErrors := m.blockConverter.markdownToBlocks(path, req.GetMode().String())
	if !allErrors.IsEmpty() && req.Mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
		if req.Mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
			return &converter.Response{Error: allErrors}
		}
	}

	if len(files) == 0 {
		allErrors.Add(path, fmt.Errorf("couldn't found md files"))
		return &converter.Response{Error: allErrors}
	}

	for name, file := range files {
		// index links in the root csv file
		if !file.IsRootFile || !strings.EqualFold(filepath.Ext(name), ".csv") {
			continue
		}

		ext := filepath.Ext(name)
		csvDir := strings.TrimSuffix(name, ext)

		for targetName, targetFile := range files {
			fileExt := filepath.Ext(targetName)
			if filepath.Dir(targetName) == csvDir && strings.EqualFold(fileExt, ".md") {
				targetFile.HasInboundLinks = true
			}
		}
	}

	var (
		emoji, title string
		details      = make(map[string]*types.Struct, 0)
	)
	for name, file := range files {
		if strings.EqualFold(filepath.Ext(name), ".md") || strings.EqualFold(filepath.Ext(name), ".csv") {
			file.PageID = uuid.New().String()
			if len(file.ParsedBlocks) > 0 {
				if text := file.ParsedBlocks[0].GetText(); text != nil && text.Style == model.BlockContentText_Header1 {
					title = text.Text
					titleParts := strings.SplitN(title, " ", 2)

					// only select the first rune to see if it looks like emoji
					if len(titleParts) == 2 && emojiAproxRegexp.MatchString(string([]rune(titleParts[0])[0:1])) {
						// first symbol is emoji - just use it all before the space
						emoji = titleParts[0]
						title = titleParts[1]
					}
					// remove title block
					file.ParsedBlocks = file.ParsedBlocks[1:]
				}
			}

			if emoji == "" {
				emoji = slice.GetRandomString(articleIcons, name)
			}

			if title == "" {
				title = strings.TrimSuffix(filepath.Base(name), filepath.Ext(name))
				titleParts := strings.Split(title, " ")
				title = strings.Join(titleParts[:len(titleParts)-1], " ")
			}

			file.Title = title
			// FIELD-BLOCK
			fields := map[string]*types.Value{
				bundle.RelationKeyName.String():       pbtypes.String(title),
				bundle.RelationKeyIconEmoji.String():  pbtypes.String(emoji),
				bundle.RelationKeySource.String():     pbtypes.String(file.Source),
				bundle.RelationKeyIsFavorite.String(): pbtypes.Bool(true),
			}

			details[name] = &types.Struct{Fields: fields}
			emoji = ""
			title = ""
		}
	}

	for name, file := range files {

		if file.PageID == "" {
			// file is not a page
			continue
		}

		file.ParsedBlocks = m.processFieldBlockIfItIs(file.ParsedBlocks, files)

		for _, block := range file.ParsedBlocks {
			if link := block.GetLink(); link != nil {
				target, err := url.PathUnescape(link.TargetBlockId)
				if err != nil {
					allErrors.Add(name, err)
					if req.Mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
						return &converter.Response{Error: allErrors}
					}
					log.Warnf("err while url.PathUnescape: %s \n \t\t\t url: %s", err, link.TargetBlockId)
					target = link.TargetBlockId
				}

				if files[target] != nil {
					link.TargetBlockId = files[target].PageID
					files[target].HasInboundLinks = true
				}

			} else if text := block.GetText(); text != nil && text.Marks != nil && len(text.Marks.Marks) > 0 {
				for _, mark := range text.Marks.Marks {
					if mark.Type != model.BlockContentTextMark_Mention && mark.Type != model.BlockContentTextMark_Object {
						continue
					}

					if targetFile, exists := files[mark.Param]; exists {
						mark.Param = targetFile.PageID
					}
				}
			}
		}
	}

	for name, file := range files {
		if file.IsRootFile && strings.EqualFold(filepath.Ext(name), ".csv") {
			details[name].Fields[bundle.RelationKeyIsFavorite.String()] = pbtypes.Bool(true)
			file.ParsedBlocks = m.convertCsvToLinks(name, files)
		}

		if file.PageID == "" {
			// file is not a page
			continue
		}

		var blocks = make([]*model.Block, 0, len(file.ParsedBlocks))
		for i, b := range file.ParsedBlocks {
			if f := b.GetFile(); f != nil && strings.EqualFold(filepath.Ext(f.Name), ".csv") {
				if csvFile, exists := files[f.Name]; exists {
					csvFile.HasInboundLinks = true
				} else {
					continue
				}

				csvInlineBlocks := m.convertCsvToLinks(f.Name, files)
				blocks = append(blocks, csvInlineBlocks...)
			} else {
				blocks = append(blocks, file.ParsedBlocks[i])
			}
		}

		file.ParsedBlocks = blocks
	}

	// process file blocks
	for _, file := range files {
		if file.PageID == "" {
			// not a page
			continue
		}

		for _, b := range file.ParsedBlocks {
			if b.Id == "" {
				b.Id = bson.NewObjectId().Hex()
			}
		}
	}

	snapshots := make([]*converter.Snapshot, 0)
	for name, file := range files {
		if file.PageID == "" {
			// file is not a page
			continue
		}
		snapshots = append(snapshots, &converter.Snapshot{
			Id:       file.PageID,
			SbType:   smartblock.SmartBlockTypePage,
			FileName: name,
			Snapshot: &model.SmartBlockSnapshotBase{
				Blocks:      file.ParsedBlocks,
				Details:     details[name],
				ObjectTypes: []string{bundle.TypeKeyPage.URL()},
			},
		})
	}
	if len(snapshots) == 0 {
		allErrors.Add(path, fmt.Errorf("failed to get snaphots from path, no md files"))
	}

	if allErrors.IsEmpty() {
		return &converter.Response{Snapshots: snapshots}
	}
	return &converter.Response{Snapshots: snapshots, Error: allErrors}
}

func (m *Markdown) convertCsvToLinks(csvFileName string, files map[string]*FileInfo) (blocks []*model.Block) {
	ext := filepath.Ext(csvFileName)
	csvDir := strings.TrimSuffix(csvFileName, ext)

	for name, file := range files {
		fileExt := filepath.Ext(name)
		if filepath.Dir(name) == csvDir && strings.EqualFold(fileExt, ".md") {
			file.HasInboundLinks = true
			fields := make(map[string]*types.Value)
			fields[bundle.RelationKeyName.String()] = &types.Value{
				Kind: &types.Value_StringValue{StringValue: file.Title},
			}

			blocks = append(blocks, &model.Block{
				Id: bson.NewObjectId().Hex(),
				Content: &model.BlockContentOfLink{
					Link: &model.BlockContentLink{
						TargetBlockId: file.PageID,
						Style:         model.BlockContentLink_Page,
						Fields: &types.Struct{
							Fields: fields,
						},
					},
				},
			})
		}
	}

	return blocks
}

func (m *Markdown) processFieldBlockIfItIs(blocks []*model.Block, files map[string]*FileInfo) (blocksOut []*model.Block) {
	if len(blocks) < 1 || blocks[0].GetText() == nil {
		return blocks
	}
	blocksOut = blocks

	txt := blocks[0].GetText().Text
	if txt == "" ||
		(blocks[0].GetText().Marks != nil && len(blocks[0].GetText().Marks.Marks) > 0) {
		// fields can't have a markup
		return blocks
	}

	potentialPairs := strings.Split(txt, "\n")

	var text string
	var marks []*model.BlockContentTextMark
	for _, pair := range potentialPairs {
		if text != "" {
			text += "\n"
		}
		if len(pair) <= 3 || pair[len(pair)-3:] != ".md" {
			text += pair
			continue
		}

		keyVal := strings.SplitN(pair, ":", 2)
		if len(keyVal) < 2 {
			text += pair
			continue
		}

		potentialFileNames := strings.Split(keyVal[1], ",")
		text += keyVal[0] + ": "

		for potentialFileNameIndex, potentialFileName := range potentialFileNames {
			potentialFileName, _ = url.PathUnescape(potentialFileName)
			potentialFileName = strings.ReplaceAll(potentialFileName, `"`, "")
			if potentialFileNameIndex != 0 {
				text += ", "
			}
			shortPath := ""
			id := m.getIdFromPath(potentialFileName)
			for name, _ := range files {
				if m.getIdFromPath(name) == id {
					shortPath = name
					break
				}
			}

			file := files[shortPath]

			if file == nil || len(file.PageID) == 0 {
				text += potentialFileName
				log.Errorf("target file not found:", shortPath, potentialFileName)
			} else {
				log.Debug("target file found:", file.PageID, shortPath)
				file.HasInboundLinks = true
				if file.Title == "" {
					// shouldn't be a case
					file.Title = shortPath
				}

				text += file.Title
				marks = append(marks, &model.BlockContentTextMark{
					Range: &model.Range{int32(len(text) - len(file.Title)), int32(len(text))},
					Type:  model.BlockContentTextMark_Mention,
					Param: file.PageID,
				})
			}
		}
	}

	if len(marks) > 0 {
		blockText := blocks[0].GetText()
		blockText.Text = text
		blockText.Marks = &model.BlockContentTextMarks{Marks: marks}
	}

	return blocksOut
}

func (m *Markdown) getIdFromPath(path string) (id string) {
	a := strings.Split(path, " ")
	b := a[len(a)-1]
	if len(b) < 3 {
		return ""
	}
	return b[:len(b)-3]
}

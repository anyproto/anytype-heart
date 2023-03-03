package markdown

import (
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/globalsign/mgo/bson"
	"github.com/gogo/protobuf/types"
	"github.com/textileio/go-threads/core/thread"

	"github.com/anytypeio/go-anytype-middleware/core/block/import/converter"
	"github.com/anytypeio/go-anytype-middleware/core/block/process"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/threads"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/anytypeio/go-anytype-middleware/util/slice"
)

var (
	emojiAproxRegexp = regexp.MustCompile(`[\x{2194}-\x{329F}\x{1F000}-\x{1FADF}]`)

	log          = logging.Logger("markdown-import")
	articleIcons = []string{"📓", "📕", "📗", "📘", "📙", "📖", "📔", "📒", "📝", "📄", "📑"}
)

const numberOfStages = 9 // 8 cycles to get snaphots and 1 cycle to create objects

func init() {
	converter.RegisterFunc(New)
}

type Markdown struct {
	blockConverter *mdConverter
}

const Name = "Markdown"

func New(s core.Service) converter.Converter {
	return &Markdown{blockConverter: newMDConverter(s)}
}

func (m *Markdown) Name() string {
	return Name
}

func (m *Markdown) GetParams(req *pb.RpcObjectImportRequest) []string {
	if p := req.GetMarkdownParams(); p != nil {
		return p.Path
	}

	return nil
}

func (m *Markdown) GetImage() ([]byte, int64, int64, error) {
	return nil, 0, 0, nil
}

func (m *Markdown) GetSnapshots(req *pb.RpcObjectImportRequest,
	progress *process.Progress) (*converter.Response, converter.ConvertError) {
	path := m.GetParams(req)

	if len(path) == 0 {
		return nil, converter.NewFromError("", fmt.Errorf("no path to files were provided"))
	}

	var (
		allSnapshots []*converter.Snapshot
		allErrors    = converter.NewError()
	)
	for _, p := range path {
		sn, cancelError := m.processImportPath(req, progress, p, allErrors)
		if !cancelError.IsEmpty() {
			return nil, cancelError
		}
		if !allErrors.IsEmpty() && req.Mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
			return nil, allErrors
		}
		allSnapshots = append(allSnapshots, sn...)
	}

	if len(allSnapshots) == 0 {
		allErrors.Add("", fmt.Errorf("failed to get snaphots from path, no md files"))
	}

	if allErrors.IsEmpty() {
		return &converter.Response{Snapshots: allSnapshots}, nil
	}

	return &converter.Response{Snapshots: allSnapshots}, allErrors
}

func (m *Markdown) processImportPath(req *pb.RpcObjectImportRequest,
	progress *process.Progress,
	p string,
	allErrors converter.ConvertError) ([]*converter.Snapshot, converter.ConvertError) {
	files, err := m.blockConverter.markdownToBlocks(p, req.GetMode().String())
	if !err.IsEmpty() {
		allErrors.Merge(err)
		if req.Mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
			return nil, nil
		}
	}

	if len(files) == 0 {
		log.Errorf("couldn't found md files, path: %s", p)
		return nil, nil
	}
	progress.SetTotal(int64(numberOfStages * len(files)))

	progress.SetProgressMessage("Start linking database file with pages")

	if cancellErr := m.setInboundLinks(files, progress); cancellErr != nil {
		return nil, cancellErr
	}

	var (
		details = make(map[string]*types.Struct, 0)
	)

	progress.SetProgressMessage("Start creating blocks")

	if cancellErr := m.createThreadObject(files, progress, details, allErrors, req.Mode); cancellErr != nil {
		return nil, cancellErr
	}

	progress.SetProgressMessage("Start linking blocks")

	if cancelErr := m.createMarkdownForLink(files, progress, allErrors, req.Mode); cancelErr != nil {
		return nil, cancelErr
	}

	progress.SetProgressMessage("Start linking database with pages")

	if cancellErr := m.linkPagesWithRootFile(files, progress, details); cancellErr != nil {
		return nil, cancellErr
	}

	progress.SetProgressMessage("Start creating file blocks")

	childBlocks, cancelErr := m.fillEmptyBlocks(files, progress)

	if cancelErr != nil {
		return nil, cancelErr
	}

	progress.SetProgressMessage("Start creating link blocks")

	if cancellErr := m.addLinkBlocks(files, progress); cancellErr != nil {
		return nil, cancellErr
	}

	progress.SetProgressMessage("Start creating root blocks")

	if cancelErr = m.addChildBlocks(files, progress, childBlocks); cancelErr != nil {
		return nil, cancelErr
	}
	progress.SetProgressMessage("Start creating snaphots")

	var snapshots []*converter.Snapshot
	if snapshots, cancelErr = m.createSnapshots(files, progress, details); cancelErr != nil {
		return nil, cancelErr
	}
	return snapshots, nil
}

func isChildBlock(blocks []string, b *model.Block) bool {
	for _, block := range blocks {
		if b.Id == block {
			return true
		}
	}
	return false
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

func (m *Markdown) setInboundLinks(files map[string]*FileInfo, progress *process.Progress) converter.ConvertError {
	for name, file := range files {
		if err := progress.TryStep(1); err != nil {
			cancellError := converter.NewFromError(name, err)
			return cancellError
		}

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

	return nil
}

func (m *Markdown) linkPagesWithRootFile(files map[string]*FileInfo,
	progress *process.Progress,
	details map[string]*types.Struct) converter.ConvertError {
	for name, file := range files {
		if err := progress.TryStep(1); err != nil {
			cancellError := converter.NewFromError(name, err)
			return cancellError
		}

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

	return nil
}
func (m *Markdown) addLinkBlocks(files map[string]*FileInfo, progress *process.Progress) converter.ConvertError {
	for name, file := range files {
		if err := progress.TryStep(1); err != nil {
			cancellError := converter.NewFromError(name, err)
			return cancellError
		}

		if file.PageID == "" {
			// not a page
			continue
		}

		if file.HasInboundLinks {
			continue
		}

		file.ParsedBlocks = append(file.ParsedBlocks, &model.Block{
			Content: &model.BlockContentOfLink{
				Link: &model.BlockContentLink{
					TargetBlockId: file.PageID,
					Style:         model.BlockContentLink_Page,
					Fields:        nil,
				},
			},
		})
	}

	return nil
}

func (m *Markdown) createSnapshots(files map[string]*FileInfo,
	progress *process.Progress,
	details map[string]*types.Struct) ([]*converter.Snapshot, converter.ConvertError) {
	snapshots := make([]*converter.Snapshot, 0)

	for name, file := range files {
		if err := progress.TryStep(1); err != nil {
			cancellError := converter.NewFromError(name, err)
			return nil, cancellError
		}

		if file.PageID == "" {
			// file is not a page
			continue
		}

		snapshots = append(snapshots, &converter.Snapshot{
			Id:       file.PageID,
			FileName: name,
			Snapshot: &model.SmartBlockSnapshotBase{
				Blocks:      file.ParsedBlocks,
				Details:     details[name],
				ObjectTypes: pbtypes.GetStringList(details[name], bundle.RelationKeyType.String()),
			},
		})
	}

	return snapshots, nil
}

func (m *Markdown) addChildBlocks(files map[string]*FileInfo,
	progress *process.Progress,
	childBlocks []string) converter.ConvertError {
	for name, file := range files {
		if err := progress.TryStep(1); err != nil {
			cancelError := converter.NewFromError(name, err)
			return cancelError
		}

		if file.PageID == "" {
			// not a page
			continue
		}

		var childrenIds = make([]string, len(file.ParsedBlocks))
		for _, b := range file.ParsedBlocks {
			if isChildBlock(childBlocks, b) {
				continue
			}
			childrenIds = append(childrenIds, b.Id)
		}

		file.ParsedBlocks = append(file.ParsedBlocks, &model.Block{
			Id:          file.PageID,
			ChildrenIds: childrenIds,
			Content:     &model.BlockContentOfSmartblock{},
		})
	}
	return nil
}

func (m *Markdown) createMarkdownForLink(files map[string]*FileInfo,
	progress *process.Progress,
	allErrors converter.ConvertError,
	mode pb.RpcObjectImportRequestMode) converter.ConvertError {
	for name, file := range files {
		if err := progress.TryStep(1); err != nil {
			cancellError := converter.NewFromError(name, err)
			return cancellError
		}

		if file.PageID == "" {
			// file is not a page
			continue
		}

		file.ParsedBlocks = m.processFieldBlockIfItIs(file.ParsedBlocks, files)

		for _, block := range file.ParsedBlocks {
			if link := block.GetLink(); link != nil {
				target, err := url.PathUnescape(link.TargetBlockId)
				if err != nil && mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
					allErrors.Add(name, err)
					return allErrors
				}

				if err != nil {
					allErrors.Add(name, err)
					log.Warnf("err while url.PathUnescape: %s \n \t\t\t url: %s", err, link.TargetBlockId)
					target = link.TargetBlockId
				}

				if files[target] != nil {
					link.TargetBlockId = files[target].PageID
					files[target].HasInboundLinks = true
				}

				continue
			}

			if text := block.GetText(); text != nil && text.Marks != nil && len(text.Marks.Marks) > 0 {
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

	return nil
}

func (m *Markdown) fillEmptyBlocks(files map[string]*FileInfo,
	progress *process.Progress) ([]string, converter.ConvertError) {
	// process file blocks
	childBlocks := make([]string, 0)
	for name, file := range files {
		if err := progress.TryStep(1); err != nil {
			cancellError := converter.NewFromError(name, err)
			return nil, cancellError
		}

		if file.PageID == "" {
			continue
		}

		for _, b := range file.ParsedBlocks {
			if len(b.ChildrenIds) != 0 {
				childBlocks = append(childBlocks, b.ChildrenIds...)
			}
			if b.Id == "" {
				b.Id = bson.NewObjectId().Hex()
			}
		}
	}
	return childBlocks, nil
}

func (m *Markdown) createThreadObject(files map[string]*FileInfo,
	progress *process.Progress,
	details map[string]*types.Struct,
	allErrors converter.ConvertError,
	mode pb.RpcObjectImportRequestMode) converter.ConvertError {
	for name, file := range files {
		if err := progress.TryStep(1); err != nil {
			cancellError := converter.NewFromError(name, err)
			return cancellError
		}

		if strings.EqualFold(filepath.Ext(name), ".md") || strings.EqualFold(filepath.Ext(name), ".csv") {
			tid, err := threads.ThreadCreateID(thread.AccessControlled, smartblock.SmartBlockTypePage)
			if err != nil {
				allErrors.Add(name, err)
			}

			if err != nil && mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
				return allErrors
			}

			file.PageID = tid.String()

			m.setDetails(file, name, details)
		}
	}

	return nil
}

func (m *Markdown) setDetails(file *FileInfo, name string, details map[string]*types.Struct) {
	var title, emoji string
	if len(file.ParsedBlocks) > 0 {
		title, emoji = m.extractTitleAndEmojiFromBlock(file)
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
}

func (m *Markdown) extractTitleAndEmojiFromBlock(file *FileInfo) (string, string) {
	var title, emoji string
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

	return title, emoji
}

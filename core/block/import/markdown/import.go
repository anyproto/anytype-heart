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

	"github.com/anyproto/anytype-heart/core/block/collection"
	"github.com/anyproto/anytype-heart/core/block/import/converter"
	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

var (
	emojiAproxRegexp = regexp.MustCompile(`[\x{2194}-\x{329F}\x{1F000}-\x{1FADF}]`)
	log              = logging.Logger("markdown-import")
)

const numberOfStages = 9 // 8 cycles to get snaphots and 1 cycle to create objects

type Markdown struct {
	blockConverter *mdConverter
	service        *collection.Service
}

const (
	Name               = "Markdown"
	rootCollectionName = "Markdown Import"
)

func New(tempDirProvider core.TempDirProvider, service *collection.Service) converter.Converter {
	return &Markdown{blockConverter: newMDConverter(tempDirProvider), service: service}
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

func (m *Markdown) GetSnapshots(req *pb.RpcObjectImportRequest, progress process.Progress) (*converter.Response, converter.ConvertError) {
	paths := m.GetParams(req)
	if len(paths) == 0 {
		return nil, nil
	}
	var (
		allSnapshots []*converter.Snapshot
		allErrors    = converter.NewError()
	)
	for _, path := range paths {
		snapshots, convertError := m.getSnapshots(req, progress, path, allErrors)
		if !convertError.IsEmpty() {
			return nil, convertError
		}
		if !allErrors.IsEmpty() && req.Mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
			return nil, allErrors
		}
		allSnapshots = append(allSnapshots, snapshots...)
	}

	if len(allSnapshots) == 0 {
		allErrors.Add("", fmt.Errorf("failed to get snaphots from path, no md files"))
	}

	allSnapshots, err := m.createRootCollection(allSnapshots)
	if err != nil {
		allErrors.Add(rootCollectionName, err)
		if req.Mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
			return nil, allErrors
		}
	}

	if allErrors.IsEmpty() {
		return &converter.Response{Snapshots: allSnapshots}, nil
	}
	return &converter.Response{Snapshots: allSnapshots}, allErrors
}

func (m *Markdown) createRootCollection(allSnapshots []*converter.Snapshot) ([]*converter.Snapshot, error) {
	targetObjects := m.getObjectIDs(allSnapshots)
	rootCollection := converter.NewRootCollection(m.service)
	rootCol, err := rootCollection.AddObjects(rootCollectionName, targetObjects)
	if err != nil {
		return nil, err
	}

	if rootCol != nil {
		allSnapshots = append(allSnapshots, rootCol)
	}
	return allSnapshots, nil
}

func (m *Markdown) getSnapshots(req *pb.RpcObjectImportRequest, progress process.Progress, path string, allErrors converter.ConvertError) ([]*converter.Snapshot, converter.ConvertError) {
	files, e := m.blockConverter.markdownToBlocks(path, req.GetMode().String())
	if !e.IsEmpty() && req.Mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
		if req.Mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
			return nil, e
		}
		allErrors.Merge(e)
	}

	if len(files) == 0 {
		allErrors.Add(path, fmt.Errorf("couldn't found md files"))
		return nil, nil
	}
	progress.SetTotal(int64(numberOfStages * len(files)))

	if cancelErr := m.setInboundLinks(files, progress); cancelErr != nil {
		return nil, cancelErr
	}
	details := make(map[string]*types.Struct, 0)

	if cancelErr := m.setNewID(files, progress, details); cancelErr != nil {
		return nil, cancelErr
	}

	if cancelErr := m.addLinkToObjectBlocks(files, progress, allErrors, req.Mode); cancelErr != nil {
		return nil, cancelErr
	}

	if cancelErr := m.linkPagesWithRootFile(files, progress); cancelErr != nil {
		return nil, cancelErr
	}

	childBlocks, cancelErr := m.fillEmptyBlocks(files, progress)

	if cancelErr != nil {
		return nil, cancelErr
	}

	if cancelErr = m.addLinkBlocks(files, progress); cancelErr != nil {
		return nil, cancelErr
	}

	if cancelErr = m.addChildBlocks(files, progress, childBlocks); cancelErr != nil {
		return nil, cancelErr
	}

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
				log.Errorf("target file not found")
			} else {
				log.Debug("target file found:", file.PageID)
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

func (m *Markdown) setInboundLinks(files map[string]*FileInfo, progress process.Progress) converter.ConvertError {
	progress.SetProgressMessage("Start linking database file with pages")
	for name, file := range files {
		if err := progress.TryStep(1); err != nil {
			return converter.NewCancelError(name, err)
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
	progress process.Progress) converter.ConvertError {
	progress.SetProgressMessage("Start linking database with pages")
	for name, file := range files {
		if err := progress.TryStep(1); err != nil {
			return converter.NewCancelError(name, err)
		}

		if file.IsRootFile && strings.EqualFold(filepath.Ext(name), ".csv") {
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
func (m *Markdown) addLinkBlocks(files map[string]*FileInfo, progress process.Progress) converter.ConvertError {
	progress.SetProgressMessage("Start creating link blocks")
	for name, file := range files {
		if err := progress.TryStep(1); err != nil {
			return converter.NewCancelError(name, err)
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
	progress process.Progress,
	details map[string]*types.Struct) ([]*converter.Snapshot, converter.ConvertError) {
	snapshots := make([]*converter.Snapshot, 0)
	progress.SetProgressMessage("Start creating snapshots")
	for name, file := range files {
		if err := progress.TryStep(1); err != nil {
			return nil, converter.NewCancelError(name, err)
		}

		if file.PageID == "" {
			// file is not a page
			continue
		}

		snapshots = append(snapshots, &converter.Snapshot{
			Id:       file.PageID,
			FileName: name,
			SbType:   smartblock.SmartBlockTypePage,
			Snapshot: &pb.ChangeSnapshot{Data: &model.SmartBlockSnapshotBase{
				Blocks:      file.ParsedBlocks,
				Details:     details[name],
				ObjectTypes: []string{bundle.TypeKeyPage.URL()},
			}},
		})
	}

	return snapshots, nil
}

func (m *Markdown) addChildBlocks(files map[string]*FileInfo,
	progress process.Progress,
	childBlocks []string) converter.ConvertError {
	progress.SetProgressMessage("Start creating root blocks")
	for name, file := range files {
		if err := progress.TryStep(1); err != nil {
			return converter.NewCancelError(name, err)
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

func (m *Markdown) addLinkToObjectBlocks(files map[string]*FileInfo,
	progress process.Progress,
	allErrors converter.ConvertError,
	mode pb.RpcObjectImportRequestMode) converter.ConvertError {
	progress.SetProgressMessage("Start linking blocks")
	for name, file := range files {
		if err := progress.TryStep(1); err != nil {
			return converter.NewCancelError(name, err)
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
	progress process.Progress) ([]string, converter.ConvertError) {
	progress.SetProgressMessage("Start creating file blocks")
	// process file blocks
	childBlocks := make([]string, 0)
	for name, file := range files {
		if err := progress.TryStep(1); err != nil {
			return nil, converter.NewCancelError(name, err)
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

func (m *Markdown) setNewID(
	files map[string]*FileInfo,
	progress process.Progress,
	details map[string]*types.Struct,
) converter.ConvertError {
	progress.SetProgressMessage("Start creating blocks")
	for name, file := range files {
		if err := progress.TryStep(1); err != nil {
			return converter.NewCancelError(name, err)
		}

		if strings.EqualFold(filepath.Ext(name), ".md") || strings.EqualFold(filepath.Ext(name), ".csv") {
			file.PageID = uuid.New().String()

			m.setDetails(file, name, details)
		}
	}

	return nil
}

func (m *Markdown) setDetails(file *FileInfo, fileName string, details map[string]*types.Struct) {
	var title, emoji string
	if len(file.ParsedBlocks) > 0 {
		title, emoji = m.extractTitleAndEmojiFromBlock(file)
	}
	details[fileName] = converter.GetCommonDetails(fileName, title, emoji)
	file.Title = pbtypes.GetString(details[fileName], bundle.RelationKeyName.String())
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

func (m *Markdown) getObjectIDs(snapshots []*converter.Snapshot) []string {
	targetObject := make([]string, 0, len(snapshots))
	for _, snapshot := range snapshots {
		targetObject = append(targetObject, snapshot.Id)
	}
	return targetObject
}

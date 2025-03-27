package markdown

import (
	"context"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/globalsign/mgo/bson"
	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block/collection"
	"github.com/anyproto/anytype-heart/core/block/import/common"
	"github.com/anyproto/anytype-heart/core/block/import/common/source"
	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

var (
	emojiAproxRegexp = regexp.MustCompile(`[\x{2194}-\x{329F}\x{1F000}-\x{1FADF}]`)
	log              = logging.Logger("markdown-import")
)

const numberOfStages = 9 // 8 cycles to get snapshots and 1 cycle to create objects

type Markdown struct {
	blockConverter *mdConverter
	service        *collection.Service
}

const (
	Name               = "Markdown"
	rootCollectionName = "Markdown Import"
)

func New(tempDirProvider core.TempDirProvider, service *collection.Service) common.Converter {
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

func (m *Markdown) GetSnapshots(ctx context.Context, req *pb.RpcObjectImportRequest, progress process.Progress) (*common.Response, *common.ConvertError) {
	paths := m.GetParams(req)
	if len(paths) == 0 {
		return nil, nil
	}
	allErrors := common.NewError(req.Mode)
	allSnapshots, allRootObjectsIds := m.processFiles(req, progress, paths, allErrors)
	if allErrors.ShouldAbortImport(len(paths), req.Type) {
		return nil, allErrors
	}
	allSnapshots, rootCollectionID, err := m.createRootCollection(allSnapshots, allRootObjectsIds)
	if err != nil {
		allErrors.Add(err)
		if allErrors.ShouldAbortImport(len(paths), req.Type) {
			return nil, allErrors
		}
	}

	if allErrors.IsEmpty() {
		return &common.Response{Snapshots: allSnapshots, RootCollectionID: rootCollectionID}, nil
	}
	return &common.Response{Snapshots: allSnapshots, RootCollectionID: rootCollectionID}, allErrors
}

func (m *Markdown) processFiles(req *pb.RpcObjectImportRequest, progress process.Progress, paths []string, allErrors *common.ConvertError) ([]*common.Snapshot, []string) {
	var (
		allSnapshots      []*common.Snapshot
		allRootObjectsIds []string
	)
	for _, path := range paths {
		snapshots, rootObjectsIds := m.getSnapshotsAndRootObjectsIds(req, progress, path, allErrors)
		if allErrors.ShouldAbortImport(len(paths), req.Type) {
			return nil, nil
		}
		allSnapshots = append(allSnapshots, snapshots...)
		allRootObjectsIds = append(allRootObjectsIds, rootObjectsIds...)
	}
	return allSnapshots, allRootObjectsIds
}

func (m *Markdown) createRootCollection(allSnapshots []*common.Snapshot, allRootObjectsIds []string) ([]*common.Snapshot, string, error) {
	rootCollection := common.NewImportCollection(m.service)
	settings := common.NewImportCollectionSetting(
		common.WithCollectionName(rootCollectionName),
		common.WithTargetObjects(allRootObjectsIds),
		common.WithAddDate(),
		common.WithRelations(),
	)
	rootCol, widgetSnapshot, err := rootCollection.MakeImportCollection(settings)
	if err != nil {
		return nil, "", err
	}

	var rootCollectionID string
	if rootCol != nil {
		allSnapshots = append(allSnapshots, rootCol)
		rootCollectionID = rootCol.Id
	}
	if widgetSnapshot != nil {
		allSnapshots = append(allSnapshots, widgetSnapshot)
	}
	return allSnapshots, rootCollectionID, nil
}

func (m *Markdown) getSnapshotsAndRootObjectsIds(
	req *pb.RpcObjectImportRequest,
	progress process.Progress,
	path string,
	allErrors *common.ConvertError,
) ([]*common.Snapshot, []string) {
	importSource := source.GetSource(path)
	if importSource == nil {
		return nil, nil
	}
	defer importSource.Close()
	files := m.blockConverter.markdownToBlocks(path, importSource, allErrors)
	pathsCount := len(req.GetMarkdownParams().Path)
	if allErrors.ShouldAbortImport(pathsCount, req.Type) {
		return nil, nil
	}

	progress.SetTotal(int64(numberOfStages * len(files)))
	details := make(map[string]*domain.Details, 0)

	if m.processImportStep(pathsCount, files, progress, allErrors, details, m.setInboundLinks) ||
		m.processImportStep(pathsCount, files, progress, allErrors, details, m.setNewID) ||
		m.processImportStep(pathsCount, files, progress, allErrors, details, m.addLinkToObjectBlocks) ||
		m.processImportStep(pathsCount, files, progress, allErrors, details, m.linkPagesWithRootFile) ||
		m.processImportStep(pathsCount, files, progress, allErrors, details, m.addLinkBlocks) ||
		m.processImportStep(pathsCount, files, progress, allErrors, details, m.fillEmptyBlocks) ||
		m.processImportStep(pathsCount, files, progress, allErrors, details, m.addChildBlocks) {
		return nil, nil
	}

	return m.createSnapshots(pathsCount, files, progress, details, allErrors), m.retrieveRootObjectsIds(files)
}

func (m *Markdown) processImportStep(pathCount int,
	files map[string]*FileInfo,
	progress process.Progress,
	allErrors *common.ConvertError,
	details map[string]*domain.Details,
	callback func(map[string]*FileInfo, process.Progress, map[string]*domain.Details, *common.ConvertError),
) (abortImport bool) {
	callback(files, progress, details, allErrors)
	return allErrors.ShouldAbortImport(pathCount, model.Import_Markdown)
}

func (m *Markdown) retrieveCollectionObjectsIds(csvFileName string, files map[string]*FileInfo) []string {
	ext := filepath.Ext(csvFileName)
	csvDir := strings.TrimSuffix(csvFileName, ext)
	var collectionsObjectsIds []string
	for name, file := range files {
		fileExt := filepath.Ext(name)
		if filepath.Dir(name) == csvDir && strings.EqualFold(fileExt, ".md") {
			file.HasInboundLinks = true
			collectionsObjectsIds = append(collectionsObjectsIds, file.PageID)
		}
	}

	return collectionsObjectsIds
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

func (m *Markdown) setInboundLinks(files map[string]*FileInfo, progress process.Progress, _ map[string]*domain.Details, allErrors *common.ConvertError) {
	progress.SetProgressMessage("Start linking database file with pages")
	for name := range files {
		if err := progress.TryStep(1); err != nil {
			allErrors.Add(common.ErrCancel)
			return
		}

		if !strings.EqualFold(filepath.Ext(name), ".csv") {
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
}

func (m *Markdown) linkPagesWithRootFile(files map[string]*FileInfo, progress process.Progress, _ map[string]*domain.Details, allErrors *common.ConvertError) {
	progress.SetProgressMessage("Start linking database with pages")
	for name, file := range files {
		if err := progress.TryStep(1); err != nil {
			allErrors.Add(common.ErrCancel)
			return
		}

		if file.PageID == "" {
			// file is not a page
			continue
		}

		if strings.EqualFold(filepath.Ext(name), ".csv") {
			file.CollectionsObjectsIds = m.retrieveCollectionObjectsIds(name, files)
		}

		blocks := make([]*model.Block, 0, len(file.ParsedBlocks))

		for i, b := range file.ParsedBlocks {
			if f := b.GetFile(); f != nil && strings.EqualFold(filepath.Ext(f.Name), ".csv") {
				csvFile, exists := files[f.Name]
				if !exists {
					continue
				}
				csvFile.HasInboundLinks = true
				csvFile.CollectionsObjectsIds = m.retrieveCollectionObjectsIds(f.Name, files)

				blocks = append(blocks, m.getLinkBlock(file))
			} else {
				blocks = append(blocks, file.ParsedBlocks[i])
			}
		}
		file.ParsedBlocks = blocks
	}
}

func (m *Markdown) getLinkBlock(file *FileInfo) *model.Block {
	fields := make(map[string]*types.Value)
	fields[bundle.RelationKeyName.String()] = &types.Value{
		Kind: &types.Value_StringValue{StringValue: file.Title},
	}
	return &model.Block{
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
	}
}

func (m *Markdown) addLinkBlocks(files map[string]*FileInfo, progress process.Progress, _ map[string]*domain.Details, allErrors *common.ConvertError) {
	progress.SetProgressMessage("Start creating link blocks")
	for _, file := range files {
		if err := progress.TryStep(1); err != nil {
			allErrors.Add(common.ErrCancel)
			return
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
}

func (m *Markdown) createSnapshots(
	pathsCount int,
	files map[string]*FileInfo,
	progress process.Progress,
	details map[string]*domain.Details,
	allErrors *common.ConvertError,
) []*common.Snapshot {
	snapshots := make([]*common.Snapshot, 0)
	progress.SetProgressMessage("Start creating snapshots")
	for name, file := range files {
		if err := progress.TryStep(1); err != nil {
			allErrors.Add(common.ErrCancel)
			return nil
		}

		if file.PageID == "" {
			// file is not a page
			continue
		}

		if filepath.Ext(name) == ".csv" {
			var err error
			snapshots, err = m.addCollectionSnapshot(name, file, snapshots)
			if err != nil {
				allErrors.Add(err)
				if allErrors.ShouldAbortImport(pathsCount, model.Import_Markdown) {
					return nil
				}
			}
			continue
		}
		snapshots = append(snapshots, &common.Snapshot{
			Id:       file.PageID,
			FileName: name,
			Snapshot: &common.SnapshotModel{
				SbType: smartblock.SmartBlockTypePage,
				Data: &common.StateSnapshot{
					Blocks:      file.ParsedBlocks,
					Details:     details[name],
					ObjectTypes: []string{bundle.TypeKeyPage.String()},
				}},
		})
	}

	return snapshots
}

func (m *Markdown) addCollectionSnapshot(fileName string, file *FileInfo, snapshots []*common.Snapshot) ([]*common.Snapshot, error) {
	c := common.NewImportCollection(m.service)
	settings := common.NewImportCollectionSetting(
		common.WithCollectionName(file.Title),
		common.WithTargetObjects(file.CollectionsObjectsIds),
	)
	csvCollection, _, err := c.MakeImportCollection(settings)
	if err != nil {
		return nil, err
	}
	csvCollection.Id = file.PageID
	csvCollection.FileName = fileName
	snapshots = append(snapshots, csvCollection)
	return snapshots, nil
}

func (m *Markdown) addChildBlocks(files map[string]*FileInfo, progress process.Progress, _ map[string]*domain.Details, allErrors *common.ConvertError) {
	progress.SetProgressMessage("Start creating root blocks")
	childBlocks := m.extractChildBlocks(files)
	for _, file := range files {
		if err := progress.TryStep(1); err != nil {
			allErrors.Add(common.ErrCancel)
			return
		}

		if file.PageID == "" {
			// not a page
			continue
		}

		childrenIds := make([]string, 0, len(file.ParsedBlocks))
		for _, b := range file.ParsedBlocks {
			if isChildBlock(childBlocks, b) {
				continue
			}
			childrenIds = append(childrenIds, b.Id)
		}

		file.ParsedBlocks = append(file.ParsedBlocks, &model.Block{
			Id:          file.PageID,
			ChildrenIds: childrenIds,
			Content: &model.BlockContentOfSmartblock{
				Smartblock: &model.BlockContentSmartblock{},
			},
		})
	}
}

func (m *Markdown) extractChildBlocks(files map[string]*FileInfo) []string {
	childBlocks := make([]string, 0)
	for _, file := range files {
		if file.PageID == "" {
			continue
		}

		for _, b := range file.ParsedBlocks {
			if len(b.ChildrenIds) != 0 {
				childBlocks = append(childBlocks, b.ChildrenIds...)
			}
		}
	}
	return childBlocks
}

func (m *Markdown) addLinkToObjectBlocks(files map[string]*FileInfo, progress process.Progress, _ map[string]*domain.Details, allErrors *common.ConvertError) {
	progress.SetProgressMessage("Start linking blocks")
	for _, file := range files {
		if err := progress.TryStep(1); err != nil {
			allErrors.Add(common.ErrCancel)
			return
		}

		if file.PageID == "" {
			// file is not a page
			continue
		}

		file.ParsedBlocks = m.processFieldBlockIfItIs(file.ParsedBlocks, files)

		for _, block := range file.ParsedBlocks {
			if link := block.GetLink(); link != nil {
				target, err := url.PathUnescape(link.TargetBlockId)
				if err != nil {
					log.Warnf("error while url.PathUnescape: %s", err)
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
}

func (m *Markdown) fillEmptyBlocks(files map[string]*FileInfo, progress process.Progress, _ map[string]*domain.Details, allErrors *common.ConvertError) {
	progress.SetProgressMessage("Start creating file blocks")
	// process file blocks
	for _, file := range files {
		if err := progress.TryStep(1); err != nil {
			allErrors.Add(common.ErrCancel)
			return
		}

		if file.PageID == "" {
			continue
		}

		for _, b := range file.ParsedBlocks {
			if b.Id == "" {
				b.Id = bson.NewObjectId().Hex()
			}
		}
	}
}

func (m *Markdown) setNewID(files map[string]*FileInfo, progress process.Progress, details map[string]*domain.Details, allErrors *common.ConvertError) {
	progress.SetProgressMessage("Start creating blocks")
	for name, file := range files {
		if err := progress.TryStep(1); err != nil {
			allErrors.Add(common.ErrCancel)
			return
		}

		if strings.EqualFold(filepath.Ext(name), ".md") || strings.EqualFold(filepath.Ext(name), ".csv") {
			file.PageID = bson.NewObjectId().Hex()

			m.setDetails(file, name, details)
		}
	}
}

func (m *Markdown) setDetails(file *FileInfo, fileName string, details map[string]*domain.Details) {
	var title, emoji string
	if len(file.ParsedBlocks) > 0 {
		title, emoji = m.extractTitleAndEmojiFromBlock(file)
	}
	details[fileName] = common.GetCommonDetails(fileName, title, emoji, model.ObjectType_basic)
	file.Title = details[fileName].GetString(bundle.RelationKeyName)
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

func (m *Markdown) retrieveRootObjectsIds(files map[string]*FileInfo) []string {
	var rootObjectsIds []string
	for _, file := range files {
		if file.PageID == "" {
			continue
		}
		if file.IsRootFile {
			rootObjectsIds = append(rootObjectsIds, file.PageID)
		}
	}
	return rootObjectsIds
}

func isChildBlock(blocks []string, b *model.Block) bool {
	for _, block := range blocks {
		if b.Id == block {
			return true
		}
	}
	return false
}

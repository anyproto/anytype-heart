package block

import (
	"os"

	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/core/block/history"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/base"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/gogo/protobuf/types"
)

func newDashboard(s *service) (smartBlock, error) {
	p := &dashboard{&commonSmart{s: s}}
	return p, nil
}

type dashboard struct {
	*commonSmart
}

func (p *dashboard) Init() {
	p.m.Lock()
	defer p.m.Unlock()
	if p.block.GetId() == p.s.anytype.PredefinedBlockIds().Home {
		// virtually add testpage to home screen
		p.addTestPage()
	}

	p.migratePageToLinks()
	if err := p.checkArchive(); err != nil {
		log.Infof("can't check archive: %v", err)
	}
	p.history = history.NewHistory(0)
	p.init()
}

func (p *dashboard) migratePageToLinks() {
	s := p.newState()
	for id, v := range p.versions {
		if id != p.GetId() && (v.Model().GetPage() != nil || v.Model().GetDashboard() != nil) {
			link := s.createLink(v.Model())
			if _, err := p.replace(s, id, link); err != nil {
				log.Infof("can't wrap page to link: %v", err)
			}
		}
		if link := v.Model().GetLink(); link != nil && link.TargetBlockId == testPageId {
			if !v.Virtual() {
				s.removeFromChilds(id)
				s.remove(id)
			}
		}
	}
	if _, err := s.apply(nil); err != nil {
		log.Infof("can't apply state for migrating page to link: %v", err)
	}
}

func (p *dashboard) checkArchive() (err error) {
	archiveId := p.s.anytype.PredefinedBlockIds().Archive
	if archiveId == "" {
		return
	}
	var removeId string
	for _, b := range p.versions {
		if link := b.Model().GetLink(); link != nil && link.TargetBlockId == archiveId {
			if link.Style != model.BlockContentLink_Archive {
				removeId = b.Model().Id
				break
			} else {
				return
			}
		}
	}
	s := p.newState()
	if removeId != "" {
		s.removeFromChilds(removeId)
		s.remove(removeId)
	}
	link := s.createLink(&model.Block{
		Id: archiveId,
		Content: &model.BlockContentOfDashboard{
			Dashboard: &model.BlockContentDashboard{
				Style: model.BlockContentDashboard_Archive,
			},
		},
	})
	l, err := s.create(link)
	if err != nil {
		return
	}
	root := s.get(p.GetId()).Model()
	root.ChildrenIds = append(root.ChildrenIds, l.Model().Id)
	return p.applyAndSendEventHist(s, false, false)
}

func (p *dashboard) addTestPage() {
	if os.Getenv("ANYTYPE_TESTPAGE") != "1" {
		return
	}

	p.versions[testPageId+"-link"] = base.NewVirtual(&model.Block{
		Id: testPageId + "-link",
		Content: &model.BlockContentOfLink{
			Link: &model.BlockContentLink{
				Style: model.BlockContentLink_Page,
				Fields: &types.Struct{
					Fields: map[string]*types.Value{
						"name": testStringValue("Test page"),
						"icon": testStringValue(":deciduous_tree:"),
					},
				},
				TargetBlockId: testPageId,
			},
		},
	})
	p.versions[p.block.GetId()].Model().ChildrenIds = append(p.versions[p.block.GetId()].Model().ChildrenIds, testPageId+"-link")
}

func (p *dashboard) Create(req pb.RpcBlockCreateRequest) (id string, err error) {
	// add empty text block on new page after create
	return p.commonSmart.Create(req)
}

func (p *dashboard) Type() smartBlockType {
	return smartBlockTypeDashboard
}

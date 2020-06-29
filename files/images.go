package files

import (
	"context"
	"io/ioutil"
	"time"

	cafepb "github.com/anytypeio/go-anytype-library/cafe/pb"
	"github.com/anytypeio/go-anytype-library/mill/schema/anytype"
	"github.com/anytypeio/go-anytype-library/pb/storage"
)

func (s *Service) ImageAdd(ctx context.Context, opts AddOptions) (string, map[int]*storage.FileInfo, error) {
	b, err := ioutil.ReadAll(opts.Reader)
	if err != nil {
		return "", nil, err
	}

	dir, err := s.fileBuildDirectory(ctx, b, opts.Name, opts.Plaintext, anytype.ImageNode())
	if err != nil {
		return "", nil, err
	}

	node, keys, err := s.fileAddNodeFromDirs(ctx, &storage.DirectoryList{Items: []*storage.Directory{dir}})
	if err != nil {
		return "", nil, err
	}

	nodeHash := node.Cid().String()

	s.KeysCacheMutex.Lock()
	defer s.KeysCacheMutex.Unlock()
	s.KeysCache[nodeHash] = keys.KeysByPath

	err = s.fileIndexData(ctx, node, nodeHash)
	if err != nil {
		return "", nil, err
	}

	var variantsByWidth = make(map[int]*storage.FileInfo, len(dir.Files))
	for _, f := range dir.Files {
		if f.Mill != "/image/resize" {
			continue
		}
		if v, exists := f.Meta.Fields["width"]; exists {
			variantsByWidth[int(v.GetNumberValue())] = f
		}
	}

	if s.cafe != nil {
		go func() {
			for i := 0; i <= 10; i++ {
				_, err := s.cafe.FilePin(context.Background(), &cafepb.FilePinRequest{Cid: nodeHash})
				if err != nil {
					log.Errorf("failed to pin image %s on the cafe: %s", nodeHash, err.Error())
					time.Sleep(time.Minute * time.Duration((i+1)*2))
					continue
				}
				log.Debugf("image %s pinned on cafe", nodeHash)

				break
			}
		}()
	}

	return nodeHash, variantsByWidth, nil
}

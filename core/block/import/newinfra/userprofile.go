package newinfra

import (
	"archive/zip"
	"io"

	"github.com/anytypeio/go-anytype-middleware/core/block/process"
	"github.com/anytypeio/go-anytype-middleware/pb"
)

func GetUserProfile(req *pb.RpcAccountRecoverFromLegacyBackupRequest,
	progress *process.Progress) (*pb.Profile, error) {
	archive, err := zip.OpenReader(req.Path)
	if err != nil {
		return nil, err
	}
	defer archive.Close()
	progress.SetTotal(1)

	f, err := archive.Open(profileFile)
	if err != nil {
		return nil, err
	}

	data, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}

	var profile pb.Profile

	err = profile.Unmarshal(data)
	if err != nil {
		return nil, err
	}
	progress.SetDone(1)
	return &profile, nil
}

package history

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/pb"
)

func makeHistoryRecord(id string, authorId string, groupId int64, hour int, min int) *pb.RpcHistoryVersion {
	return &pb.RpcHistoryVersion{
		Id:       id,
		AuthorId: authorId,
		Time:     time.Date(2000, 1, 2, hour, min, 30, 0, time.UTC).Unix(),
		GroupId:  groupId,
	}
}

func TestGroupVersions(t *testing.T) {
	for _, tc := range []struct {
		name  string
		input []*pb.RpcHistoryVersion
		want  []*pb.RpcHistoryVersion
	}{
		{
			name: "with versions having less than (or equal to) 5 minutes interval between each other expect same group",
			input: []*pb.RpcHistoryVersion{
				makeHistoryRecord("3", "author1", 0, 15, 19),
				makeHistoryRecord("2", "author1", 0, 15, 14),
				makeHistoryRecord("1", "author1", 0, 15, 10),
			},
			want: []*pb.RpcHistoryVersion{
				makeHistoryRecord("3", "author1", 0, 15, 19),
				makeHistoryRecord("2", "author1", 0, 15, 14),
				makeHistoryRecord("1", "author1", 0, 15, 10),
			},
		},
		{
			name: "with versions having more than 5 minutes interval between each other expect different groups",
			input: []*pb.RpcHistoryVersion{
				makeHistoryRecord("3", "author1", 0, 15, 22),
				makeHistoryRecord("2", "author1", 0, 15, 16),
				makeHistoryRecord("1", "author1", 0, 15, 10),
			},
			want: []*pb.RpcHistoryVersion{
				makeHistoryRecord("3", "author1", 0, 15, 22),
				makeHistoryRecord("2", "author1", 1, 15, 16),
				makeHistoryRecord("1", "author1", 2, 15, 10),
			},
		},
		{
			name: "with versions having different authors expect different groups no matter the time between versions",
			input: []*pb.RpcHistoryVersion{
				makeHistoryRecord("3", "author1", 0, 15, 17),
				makeHistoryRecord("2", "author2", 0, 15, 11),
				makeHistoryRecord("1", "author3", 0, 15, 10),
			},
			want: []*pb.RpcHistoryVersion{
				makeHistoryRecord("3", "author1", 0, 15, 17),
				makeHistoryRecord("2", "author2", 1, 15, 11),
				makeHistoryRecord("1", "author3", 2, 15, 10),
			},
		},
		{
			name: "complex case, group by author and time",
			input: []*pb.RpcHistoryVersion{
				makeHistoryRecord("7", "author1", 0, 15, 27),
				makeHistoryRecord("6", "author1", 0, 15, 17),
				makeHistoryRecord("5", "author1", 0, 15, 16),
				makeHistoryRecord("4", "author2", 0, 15, 15),
				makeHistoryRecord("3", "author2", 0, 15, 14),
				makeHistoryRecord("2", "author3", 0, 15, 13),
				makeHistoryRecord("1", "author3", 0, 15, 12),
				makeHistoryRecord("0", "author3", 0, 15, 0),
			},
			want: []*pb.RpcHistoryVersion{
				makeHistoryRecord("7", "author1", 0, 15, 27),
				makeHistoryRecord("6", "author1", 1, 15, 17),
				makeHistoryRecord("5", "author1", 1, 15, 16),
				makeHistoryRecord("4", "author2", 2, 15, 15),
				makeHistoryRecord("3", "author2", 2, 15, 14),
				makeHistoryRecord("2", "author3", 3, 15, 13),
				makeHistoryRecord("1", "author3", 3, 15, 12),
				makeHistoryRecord("0", "author3", 4, 15, 0),
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			groupVersions(tc.input)
			assert.Equal(t, tc.want, tc.input)
		})
	}
}

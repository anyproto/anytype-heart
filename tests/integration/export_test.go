package integration

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/core/block/export"
	"github.com/anyproto/anytype-heart/core/block/object/objectcreator"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func createPageWithFileBlock(t *testing.T, app *testApplication, filePath string) string {
	ctx := context.Background()
	objectCreator := getService[objectcreator.Service](app)

	id, _, err := objectCreator.CreateObject(ctx, app.personalSpaceId(), objectcreator.CreateObjectRequest{
		ObjectTypeKey: bundle.TypeKeyPage,
		Details: domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyName: domain.String("Page with file block"),
		}),
	})
	require.NoError(t, err)

	blockService := getService[*block.Service](app)
	sessionCtx := session.NewContext()
	fileBlockId, err := blockService.CreateBlock(sessionCtx, pb.RpcBlockCreateRequest{
		ContextId: id,
		TargetId:  id,
		Position:  model.Block_Inner,
		Block: &model.Block{
			Content: &model.BlockContentOfFile{
				File: &model.BlockContentFile{},
			},
		},
	})
	require.NoError(t, err)

	_, err = blockService.UploadBlockFile(nil, block.UploadRequest{
		RpcBlockUploadRequest: pb.RpcBlockUploadRequest{
			ContextId: id,
			BlockId:   fileBlockId,
			FilePath:  filePath,
		},
	}, "", true)
	require.NoError(t, err)
	return id
}

func TestExportFiles(t *testing.T) {
	tempDir := t.TempDir()

	ctx := context.Background()
	app := createAccountAndStartApp(t, pb.RpcObjectImportUseCaseRequest_NONE)

	t.Run("export protobuf", func(t *testing.T) {
		id := createPageWithFileBlock(t, app, "./testdata/test_image.png")

		exportService := getService[export.Export](app)
		exportPath, _, err := exportService.Export(ctx, pb.RpcObjectListExportRequest{
			SpaceId:      app.personalSpaceId(),
			Format:       model.Export_Protobuf,
			IncludeFiles: true,
			IsJson:       false,
			Zip:          false,
			Path:         tempDir,
			ObjectIds:    []string{id},
		})
		require.NoError(t, err)

		entries, err := os.ReadDir(exportPath)
		require.NoError(t, err)

		var foundPbFiles int
		for _, entry := range entries {
			if entry.IsDir() {
				files, err := os.ReadDir(filepath.Join(exportPath, entry.Name()))
				require.NoError(t, err)
				for _, file := range files {
					if filepath.Ext(file.Name()) == ".pb" {
						foundPbFiles++
					}
				}
			} else {
				if filepath.Ext(entry.Name()) == ".pb" {
					foundPbFiles++
				}
			}
		}
		// 4 objects total: Page object + Page type + File object
		require.GreaterOrEqual(t, foundPbFiles, 3)

		testImportObjectWithFileBlock(t, exportPath)
	})

	t.Run("export markdown", func(t *testing.T) {
		id := createPageWithFileBlock(t, app, "./testdata/saturn.jpg")

		exportService := getService[export.Export](app)
		exportPath, _, err := exportService.Export(ctx, pb.RpcObjectListExportRequest{
			SpaceId:      app.personalSpaceId(),
			Format:       model.Export_Markdown,
			IncludeFiles: true,
			IsJson:       false,
			Zip:          false,
			Path:         tempDir,
			ObjectIds:    []string{id},
		})
		require.NoError(t, err)

		entries, err := os.ReadDir(exportPath)
		require.NoError(t, err)

		var foundMarkdownFiles int
		for _, entry := range entries {
			if filepath.Ext(entry.Name()) == ".md" {
				foundMarkdownFiles++
			}
		}
		// Only one markdown file
		require.Equal(t, foundMarkdownFiles, 1)

		testImportFileFromMarkdown(t, exportPath)
	})
}

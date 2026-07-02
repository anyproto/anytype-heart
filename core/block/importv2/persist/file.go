package persist

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/core/block/importv2"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
)

// persistFile uploads a file object's bytes (content-addressed dedup happens
// inside the upload) or registers an already content-addressed file. The
// returned outcome id is the file object's final identity — the engine
// completes the identity future with it.
func (p *Persister) persistFile(ctx context.Context, o *importv2.Object) (Outcome, error) {
	if o.File == nil {
		return p.persistContentAddressedFile(o)
	}
	localPath, cleanup, err := p.materialize(ctx, o.File)
	if err != nil {
		return Outcome{}, importv2.Issue{
			Severity:  importv2.SeverityObjectError,
			Code:      importv2.IssueFileFetchFailed,
			SourceKey: o.SourceKey,
			Err:       err,
		}
	}
	defer cleanup()

	req := block.FileUploadRequest{
		RpcFileUploadRequest: pb.RpcFileUploadRequest{
			SpaceId:   p.spaceId,
			LocalPath: localPath,
			ImageKind: o.File.ImageKind,
		},
		ObjectOrigin:         p.origin,
		CustomEncryptionKeys: o.File.EncryptionKeys,
	}
	if localPath == "" {
		req.Url = o.File.URL
	}
	// Classify BEFORE the upload can be indexed: an already-indexed id
	// after upload means the content deduped onto a pre-existing object.
	objectId, _, details, err := p.uploader.UploadFile(ctx, p.spaceId, req)
	if err != nil {
		return Outcome{}, importv2.Issue{
			Severity:  importv2.SeverityObjectError,
			Code:      importv2.IssueFileFetchFailed,
			SourceKey: o.SourceKey,
			Err:       fmt.Errorf("upload %q: %w", o.File.Name, err),
		}
	}
	p.journal.CreatedFile(objectId, p.checker.Exists(objectId))
	return Outcome{Id: objectId, Action: ActionCreated, Details: details}, nil
}

// persistContentAddressedFile handles files that already live in the content
// store (anytype exports): register keys and create/find the file object.
func (p *Persister) persistContentAddressedFile(o *importv2.Object) (Outcome, error) {
	fileId := o.Payload.Details.GetString(bundle.RelationKeyId)
	if fileId == "" {
		return Outcome{}, importv2.Issue{
			Severity:  importv2.SeverityObjectError,
			Code:      importv2.IssueInvariant,
			SourceKey: o.SourceKey,
			Message:   "file object carries neither file source nor content id",
		}
	}
	objectId, err := p.uploader.CreateFromImport(
		domain.FullFileId{SpaceId: p.spaceId, FileId: domain.FileId(fileId)}, p.origin, nil)
	if err != nil {
		return Outcome{}, importv2.Issue{
			Severity:  importv2.SeverityObjectError,
			Code:      importv2.IssueFileFetchFailed,
			SourceKey: o.SourceKey,
			Err:       fmt.Errorf("create from import %q: %w", fileId, err),
		}
	}
	p.journal.CreatedFile(objectId, p.checker.Exists(objectId))
	return Outcome{Id: objectId, Action: ActionCreated, Details: nil}, nil
}

// materialize returns a local path for the file source, spilling a streamed
// entry to the run's temp dir when the uploader needs a real file. The
// cleanup removes only what materialize itself created.
func (p *Persister) materialize(ctx context.Context, source *importv2.FileSource) (string, func(), error) {
	noop := func() {}
	if source.Path != "" {
		return source.Path, noop, nil
	}
	if source.Open == nil {
		if source.URL != "" {
			return "", noop, nil // uploader fetches the url itself
		}
		return "", noop, fmt.Errorf("file source is empty")
	}
	reader, err := source.Open(ctx)
	if err != nil {
		return "", noop, fmt.Errorf("open file source: %w", err)
	}
	defer reader.Close()

	tmp, err := os.CreateTemp(p.spillDir, "import-*-"+sanitizeBase(source.Name))
	if err != nil {
		return "", noop, fmt.Errorf("create spill file: %w", err)
	}
	_, err = io.Copy(tmp, readerWithContext{ctx: ctx, r: reader})
	closeErr := tmp.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		os.Remove(tmp.Name())
		return "", noop, fmt.Errorf("spill file source: %w", err)
	}
	return tmp.Name(), func() { os.Remove(tmp.Name()) }, nil
}

// sanitizeBase keeps only the final path element of a display name so a
// hostile name can never steer the spill path.
func sanitizeBase(name string) string {
	base := filepath.Base(filepath.FromSlash(name))
	if base == "." || base == string(filepath.Separator) {
		return "file"
	}
	return base
}

type readerWithContext struct {
	ctx context.Context
	r   io.Reader
}

func (r readerWithContext) Read(b []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(b)
}

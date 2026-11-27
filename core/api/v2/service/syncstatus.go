package v2service

// syncstatus.go implements GET /v2/spaces/{space_id}/sync and the storage
// report an upload carries (POST /v2/spaces/{space_id}/files).
//
// The status and the object counts are the ones the app is sent for the
// space (spacesyncstatus), so an agent and the app's sync widget agree, with
// one exception: the app's storage warning is not computed for this read,
// and not served if present (syncErrorName).
// The storage block is the quota the file uploader itself checks for the
// space: the limit and usage of whoever pays (the owner, or in a one-to-one
// space each member for their own files), as the backup node last reported
// them, plus the uploads it confirmed since. Nothing here waits on the node.
// The files still waiting to upload are not in that usage; they come from the
// store, taken in the order the uploader takes them, which is what lets an
// upload say "this file will not fit" before the node knows the file exists.
// Everything here is an estimate: a file's size is its original's, so an
// image's resized copies are left out.

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/syncstatus/filesyncstatus"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// storageReportMaxAge is how old the node's report may be for the read to
// recommend a plan upgrade. While reachable, the report is refreshed every
// minute; an older one is from a device that has been offline, and the plan
// may have changed since.
const storageReportMaxAge = 5 * time.Minute

// GetSyncStatus reads the space's sync status. countOnly leaves out the
// object list; offset and limit page it.
func (s *Service) GetSyncStatus(ctx context.Context, spaceId string, countOnly bool, offset, limit int) (*v2model.SyncStatus, error) {
	if err := s.ensureSpace(ctx, spaceId); err != nil {
		return nil, fmt.Errorf("get sync status: %w", err)
	}
	if s.syncStatus == nil {
		return nil, v2model.NewError(http.StatusNotImplemented, v2model.CodeNotImplemented,
			"sync status is not available on this server")
	}
	event, checked, err := s.syncStatus.SpaceSyncStatus(spaceId)
	if err != nil {
		return nil, fmt.Errorf("sync status of space %s: %w", spaceId, err)
	}
	out := &v2model.SyncStatus{
		SpaceId:            s.servedSpaceRef(ctx, spaceId),
		Status:             syncStatusName(event, checked),
		Network:            syncNetworkName(event.Network),
		Error:              syncErrorName(event.Error),
		PendingObjectCount: int(event.SyncingObjectsCounter),
		UploadingFileCount: int(event.UploadingFilesCounter),
		Warnings:           s.LoadingSpaceWarnings(ctx, spaceId),
	}
	payer := s.spacePayer(spaceId)
	refused, err := s.refusedFiles(spaceId, payer)
	if err != nil {
		return nil, fmt.Errorf("files not backed up in space %s: %w", spaceId, err)
	}
	out.NotBackedUpFileCount = refused.count
	// a local-only account confirms nothing, so its objects would list
	// forever against a count of zero
	if !countOnly && out.Network != v2model.SyncNetworkLocalOnly {
		objects, hasMore, err := s.outOfSyncObjects(spaceId, offset, limit)
		if err != nil {
			return nil, fmt.Errorf("objects out of sync in space %s: %w", spaceId, err)
		}
		out.Objects = objects
		out.HasMore = hasMore
	}
	quota := s.readQuota(spaceId, out.Network)
	var verdict storageVerdict
	if quota != nil {
		out.Storage = quota.storage(quota.waiting)
		verdict = quota.judge(refused)
	}
	out.UpgradeNeeded = verdict.short && canBuyStorage(out.Network, payer, quota, verdict)
	out.Notes = syncNotes(out, checked, payer, quota, verdict, refused)
	return out, nil
}

// payerKind is who pays for a space's storage, as far as this device knows.
type payerKind int

const (
	payerUnknown payerKind = iota // no account identity, or the membership not indexed yet
	payerCaller
	payerOther
)

// spacePayer is who pays for the space's storage: the owner, and in a
// one-to-one space each member for their own files, the derived owner not
// being a participant at all.
type spacePayer struct {
	kind     payerKind
	oneToOne bool
}

func (s *Service) spacePayer(spaceId string) spacePayer {
	if s.isOneToOne(spaceId) {
		return spacePayer{kind: payerCaller, oneToOne: true}
	}
	if s.accountId == "" {
		return spacePayer{}
	}
	participantId := domain.NewParticipantId(spaceId, s.accountId)
	details, err := s.store.SpaceIndex(spaceId).GetDetails(participantId)
	if err != nil || details.GetString(bundle.RelationKeyId) != participantId {
		return spacePayer{}
	}
	if model.ParticipantPermissions(details.GetInt64(bundle.RelationKeyParticipantPermissions)) == model.ParticipantPermissions_Owner { //nolint:gosec
		return spacePayer{kind: payerCaller}
	}
	return spacePayer{kind: payerOther}
}

// isOneToOne reports whether the space is a one-to-one space, by its view's
// current marker or the earlier one.
func (s *Service) isOneToOne(spaceId string) bool {
	details, err := s.spaceViewOf(spaceId)
	if err != nil {
		return false
	}
	return model.SpaceType(details.GetInt64(bundle.RelationKeySpaceType)) == model.SpaceType_SpaceTypeOneToOne || //nolint:gosec
		model.SpaceUxType(details.GetInt64(bundle.RelationKeySpaceUxType)) == model.SpaceUxType_OneToOne //nolint:gosec
}

// refusedFileCount is the files refused for the limit: all of them, and the
// share billed to the caller's payer.
type refusedFileCount struct {
	count int
	// payerBytes is the size of the refused files the space's payer pays
	// for: all of them, or in a one-to-one space the caller's own
	payerBytes int64
	// othersCount is the refused files of the other member of a one-to-one
	// space, which their own storage pays for
	othersCount int
}

// refusedFiles counts the files in the space the uploader refused for the
// limit: the set the app counts as not synced files (syncsubscriptions).
func (s *Service) refusedFiles(spaceId string, payer spacePayer) (refusedFileCount, error) {
	records, err := s.store.SpaceIndex(spaceId).Query(database.Query{Filters: refusedFileFilters()})
	if err != nil {
		return refusedFileCount{}, fmt.Errorf("query files refused for the limit: %w", err)
	}
	me := ""
	if s.accountId != "" {
		me = domain.NewParticipantId(spaceId, s.accountId)
	}
	out := refusedFileCount{count: len(records)}
	for _, record := range records {
		if payer.oneToOne && record.Details.GetString(bundle.RelationKeyCreator) != me {
			out.othersCount++
			continue
		}
		out.payerBytes += max(0, record.Details.GetInt64(bundle.RelationKeySizeInBytes))
	}
	return out, nil
}

func refusedFileFilters() []database.FilterRequest {
	return []database.FilterRequest{
		{
			RelationKey: bundle.RelationKeyResolvedLayout,
			Condition:   model.BlockContentDataviewFilter_In,
			Value:       domain.Int64List(domain.FileLayouts),
		},
		{
			RelationKey: bundle.RelationKeyFileBackupStatus,
			Condition:   model.BlockContentDataviewFilter_Equal,
			Value:       domain.Int64(filesyncstatus.Limited),
		},
	}
}

// outOfSyncObjects lists what is not in sync on this device, most recently
// changed first: the objects the app counts as pending, by the same filter
// (syncsubscriptions), and the files refused for the limit, which the app
// counts apart and marks with the oversized error that filter leaves out.
func (s *Service) outOfSyncObjects(spaceId string, offset, limit int) ([]v2model.PendingObject, bool, error) {
	pending := []database.FilterRequest{
		{
			RelationKey: bundle.RelationKeySyncStatus,
			Condition:   model.BlockContentDataviewFilter_In,
			Value: domain.Int64List([]domain.ObjectSyncStatus{
				domain.ObjectSyncStatusSyncing,
				domain.ObjectSyncStatusQueued,
				domain.ObjectSyncStatusError,
			}),
		},
		{
			RelationKey: bundle.RelationKeySyncError,
			Condition:   model.BlockContentDataviewFilter_NotEqual,
			Value:       domain.Int64(domain.SyncErrorOversized),
		},
	}
	records, err := s.store.SpaceIndex(spaceId).Query(database.Query{
		// the branches name no archived or deleted filter, so the store's
		// implicit ones still apply to both
		Filters: []database.FilterRequest{{
			Operator: model.BlockContentDataviewFilter_Or,
			NestedFilters: []database.FilterRequest{
				{Operator: model.BlockContentDataviewFilter_And, NestedFilters: pending},
				{Operator: model.BlockContentDataviewFilter_And, NestedFilters: refusedFileFilters()},
			},
		}},
		Sorts: []database.SortRequest{{
			RelationKey: bundle.RelationKeyLastModifiedDate,
			Type:        model.BlockContentDataviewSort_Desc,
			IncludeTime: true,
		}},
		Offset: offset,
		Limit:  limit + 1, // one extra record detects has_more without a count
	})
	if err != nil {
		return nil, false, fmt.Errorf("query: %w", err)
	}
	hasMore := len(records) > limit
	if hasMore {
		records = records[:limit]
	}
	builder, err := s.newObjectRowBuilder(spaceId, nil)
	if err != nil {
		return nil, false, fmt.Errorf("row builder: %w", err)
	}
	objects := make([]v2model.PendingObject, 0, len(records))
	for _, record := range records {
		row := builder.row(record)
		object := v2model.PendingObject{Id: row.Id, Name: row.Name, Type: row.Type}
		status := domain.ObjectSyncStatus(record.Details.GetInt64(bundle.RelationKeySyncStatus)) //nolint:gosec
		switch {
		case filesyncstatus.Status(record.Details.GetInt64(bundle.RelationKeyFileBackupStatus)) == filesyncstatus.Limited: //nolint:gosec
			object.Status = v2model.PendingObjectNotBackedUp
		case status == domain.ObjectSyncStatusQueued:
			object.Status = v2model.PendingObjectQueued
		case status == domain.ObjectSyncStatusError:
			object.Status = v2model.PendingObjectError
			if domain.SyncError(record.Details.GetInt64(bundle.RelationKeySyncError)) == domain.SyncErrorNetworkError { //nolint:gosec
				object.Error = v2model.SyncErrorNetwork
			}
		default:
			object.Status = v2model.PendingObjectSyncing
		}
		objects = append(objects, object)
	}
	return objects, hasMore, nil
}

// waitingFile is one of this account's files waiting to upload.
type waitingFile struct {
	id   string
	size int64
}

// quotaView is a space's quota and the files waiting for it, read at one
// moment.
type quotaView struct {
	limit int64
	// rawUsed is the node's own report: never counts an upload twice, but
	// misses the ones confirmed since. A recommendation to buy storage is
	// judged on it, so local bookkeeping cannot make one up.
	rawUsed int64
	// used adds the uploads confirmed since the report that no longer wait:
	// the best estimate, served and used for the per-file verdicts
	used int64
	// waiting is this account's waiting files, oldest first, without the
	// ones a report already includes
	waiting []waitingFile
	// confirmed is the uploads the node confirmed lately, by file object id
	confirmed  map[string]apicore.ConfirmedUpload
	reportedAt time.Time
	planLimit  bool
}

// readQuota reads the space's quota and the files waiting for it. It is nil
// in local-only mode, while the backup node has not reported the
// quota, and when the waiting files cannot be read: storage is a best-effort
// report, never a failure of the read or the upload it rides on.
//
// The waiting files are read FIRST. A file whose upload completes between
// the two reads is then still listed as waiting, and the quota's
// confirmations say whether to count it there or in the usage. Read the other
// way round, it would be in neither.
func (s *Service) readQuota(spaceId, network string) *quotaView {
	if s.syncStatus == nil || network == v2model.SyncNetworkLocalOnly {
		return nil
	}
	waiting, err := s.waitingFiles(spaceId)
	if err != nil {
		log.Warnf("files waiting to upload in space %s: %v", spaceId, err)
		return nil
	}
	quota, ok, err := s.syncStatus.SpaceStorage(spaceId)
	if err != nil {
		log.Warnf("storage quota of space %s: %v", spaceId, err)
		return nil
	}
	if !ok {
		return nil
	}
	isWaiting := make(map[string]bool, len(waiting))
	for _, file := range waiting {
		isWaiting[file.id] = true
	}
	used := quota.NodeUsedBytes
	for id, upload := range quota.Confirmed {
		// a reported upload is in the node's usage already; one that still
		// waits is counted once, among the waiting
		if !upload.InReport && !isWaiting[id] {
			used += upload.Bytes
		}
	}
	notReported := waiting[:0:0]
	for _, file := range waiting {
		if upload, ok := quota.Confirmed[file.id]; ok && upload.InReport {
			continue
		}
		notReported = append(notReported, file)
	}
	return &quotaView{
		limit:      quota.LimitBytes,
		rawUsed:    quota.NodeUsedBytes,
		used:       used,
		waiting:    notReported,
		confirmed:  quota.Confirmed,
		reportedAt: quota.ReportedAt,
		planLimit:  quota.PlanLimit,
	}
}

// appendNew appends to files the extra ones it does not list yet and skip
// does not name.
func appendNew(files, extra []waitingFile, skip map[string]bool) []waitingFile {
	listed := make(map[string]bool, len(files))
	for _, file := range files {
		listed[file.id] = true
	}
	out := append([]waitingFile{}, files...)
	for _, file := range extra {
		if file.id == "" || (!listed[file.id] && !skip[file.id]) {
			out = append(out, file)
		}
	}
	return out
}

// waitingFiles lists this account's files in the space that wait to upload,
// the oldest first: the set the app counts as uploading files
// (syncsubscriptions), and the archived ones too, which still upload. The uploader takes them in the order it scheduled
// them, which is the order they were added unless a failed attempt pushed
// one back; that order is not readable here. Files waiting in the payer's
// other spaces, and other members' uploads, are left out.
func (s *Service) waitingFiles(spaceId string) ([]waitingFile, error) {
	filters := []database.FilterRequest{
		{
			RelationKey: bundle.RelationKeyResolvedLayout,
			Condition:   model.BlockContentDataviewFilter_In,
			Value:       domain.Int64List(domain.FileLayouts),
		},
		{
			RelationKey: bundle.RelationKeyFileBackupStatus,
			Condition:   model.BlockContentDataviewFilter_In,
			Value:       domain.Int64List([]filesyncstatus.Status{filesyncstatus.Queued, filesyncstatus.Syncing}),
		},
		// archiving a file does not stop its upload; naming the filter turns
		// the store's implicit "not archived" off
		{RelationKey: bundle.RelationKeyIsArchived, Condition: model.BlockContentDataviewFilter_None},
	}
	if s.accountId != "" {
		filters = append(filters, database.FilterRequest{
			RelationKey: bundle.RelationKeyCreator,
			Condition:   model.BlockContentDataviewFilter_Equal,
			Value:       domain.String(domain.NewParticipantId(spaceId, s.accountId)),
		})
	}
	records, err := s.store.SpaceIndex(spaceId).Query(database.Query{
		Filters: filters,
		Sorts: []database.SortRequest{{
			RelationKey: bundle.RelationKeyCreatedDate,
			Type:        model.BlockContentDataviewSort_Asc,
			IncludeTime: true,
		}},
	})
	if err != nil {
		return nil, fmt.Errorf("query files waiting to upload: %w", err)
	}
	files := make([]waitingFile, 0, len(records))
	for _, record := range records {
		files = append(files, waitingFile{
			id:   record.Details.GetString(bundle.RelationKeyId),
			size: max(0, record.Details.GetInt64(bundle.RelationKeySizeInBytes)),
		})
	}
	return files, nil
}

// admit takes the files in order the way the uploader does, from used: a
// file uploads when it fits in what is left, and is refused otherwise
// without taking any room, so a large refused file does not hold up a small
// one behind it.
func (q *quotaView) admit(files []waitingFile, used int64) (admittedBytes int64, refused map[int]bool) {
	free := q.limit - used
	for i, file := range files {
		if file.size <= free {
			free -= file.size
			admittedBytes += file.size
			continue
		}
		if refused == nil {
			refused = map[int]bool{}
		}
		refused[i] = true
	}
	return admittedBytes, refused
}

// storage is the served block, with files as the waiting ones.
func (q *quotaView) storage(files []waitingFile) *v2model.Storage {
	var pending int64
	for _, file := range files {
		pending += file.size
	}
	admitted, _ := q.admit(files, q.used)
	return &v2model.Storage{
		UsedBytes:    q.used,
		PendingBytes: pending,
		LimitBytes:   q.limit,
		LeftBytes:    max(0, q.limit-q.used-admitted),
	}
}

// storageVerdict is what the quota says about the space.
type storageVerdict struct {
	// short is a shortage seen in the node's own report, not forecast: the
	// limit is reached (or is zero), or the files refused for it do not fit
	// in what is left
	short bool
	// waitingDoNotFit is a forecast: some waiting files will be refused
	waitingDoNotFit bool
	// roomForRefused is the payer's refused files fitting now: they upload
	// once the device that holds them retries
	roomForRefused bool
	// fresh is the report recent enough to recommend buying storage on
	fresh bool
}

// rawLeft is the room the node's own report shows, before any waiting file:
// a waiting file may already be in that report, so taking it off again could
// make up a shortage.
func (q *quotaView) rawLeft() int64 {
	return max(0, q.limit-q.rawUsed)
}

func (q *quotaView) judge(refused refusedFileCount) storageVerdict {
	admitted, refusedWaiting := q.admit(q.waiting, q.used)
	left := max(0, q.limit-q.used-admitted)
	return storageVerdict{
		short:           q.rawUsed >= q.limit || refused.payerBytes > q.rawLeft(),
		waitingDoNotFit: len(refusedWaiting) > 0,
		roomForRefused:  refused.payerBytes > 0 && refused.payerBytes <= left,
		fresh:           time.Since(q.reportedAt) <= storageReportMaxAge,
	}
}

// canBuyStorage is whether more storage can be recommended for purchase at
// all: the caller pays for the space, on the Anytype network, by a current
// report, and the account report shows the limit is the plan's, so an
// upgrade raises it. Purchase is advised only then, in upgrade_needed and in
// prose alike.
func canBuyStorage(network string, payer spacePayer, quota *quotaView, verdict storageVerdict) bool {
	return quota != nil && verdict.fresh && quota.planLimit &&
		payer.kind == payerCaller && network == v2model.SyncNetworkAnytype
}

// uploadQuota is what an upload's storage report is judged by, read before
// the upload: read after, a fast upload could already be in the usage and
// still read as waiting, and count twice.
type uploadQuota struct {
	network string
	payer   spacePayer
	quota   *quotaView
	// ahead is the uploads to the space in flight before this one, taken
	// before the store was read: one that finishes meanwhile is then in the
	// store, or still here, never in neither
	ahead []*inFlightUpload
}

func (s *Service) readUploadQuota(spaceId string, ahead []*inFlightUpload) *uploadQuota {
	if s.syncStatus == nil {
		return nil
	}
	network := syncNetworkName(s.syncStatus.Network())
	quota := s.readQuota(spaceId, network)
	if quota == nil {
		return nil
	}
	return &uploadQuota{network: network, payer: s.spacePayer(spaceId), quota: quota, ahead: ahead}
}

// addUploadStorage reports on an upload result the space's quota with the
// uploaded file counted, and whether the file fits. size is its size;
// backup and creator are the stored file's, empty for a dry run, which
// judges the bytes as new. A report it cannot make is left out: the upload
// itself succeeded.
func (s *Service) addUploadStorage(spaceId string, before *uploadQuota, result *v2model.FileUploadResult, size int64, backup filesyncstatus.Status, creator string) {
	if before == nil {
		return
	}
	quota := before.quota
	// the uploads ahead as they stand now; one the node confirmed is in the
	// usage already, and one of the same bytes that resolved to this very
	// file is found below as this file's place
	ahead, unknownAhead := s.uploads.resolved(before.ahead)
	skip := map[string]bool{}
	for id := range quota.confirmed {
		skip[id] = true
	}
	files := appendNew(quota.waiting, ahead, skip)
	_, confirmed := quota.confirmed[result.Id]
	if backup == filesyncstatus.Synced || (result.Id != "" && confirmed) {
		// bytes the node already holds need no room
		result.Storage = quota.storage(files)
		result.OverLimit = new(bool)
		return
	}
	// in a one-to-one space an existing file of the other member's is billed
	// to them, whose quota this device does not see
	if before.payer.oneToOne && creator != "" && creator != domain.NewParticipantId(spaceId, s.accountId) {
		result.Storage = quota.storage(files)
		return
	}
	position := -1
	for i, file := range files {
		if result.Id != "" && file.id == result.Id {
			position = i // an existing waiting file keeps its place
		}
	}
	if position < 0 {
		files = append(files, waitingFile{id: result.Id, size: max(0, size)})
		position = len(files) - 1
	}
	result.Storage = quota.storage(files)
	_, refused := quota.admit(files, quota.used)
	overLimit := refused[position]
	if !overLimit && unknownAhead {
		// an upload ahead of unknown size may yet take the room
		return
	}
	result.OverLimit = &overLimit
	if !overLimit {
		return
	}
	fresh := time.Since(quota.reportedAt) <= storageReportMaxAge
	prefix, verb := "This file does not fit", "stays"
	if result.DryRun {
		prefix, verb = "This file would not fit", "would stay"
	}
	if !fresh {
		// what the last report said, not what to do: the plan may have
		// changed since
		result.Note = "By the backup node's last report, " + strings.ToLower(prefix[:1]) + prefix[1:] +
			" in the storage left, so it " + verb + " on this device, not backed up, until there is room."
		return
	}
	// purchase is advised only when the node's own figure, before any waiting
	// file, has no room for this one: local bookkeeping alone never does
	canBuy := size > quota.rawLeft() && canBuyStorage(before.network, before.payer, quota, storageVerdict{fresh: fresh})
	result.Note = prefix + " in the storage left, so it " + verb + " on this device, not backed up, until there is room. " +
		storageRemedy(before.network, before.payer, canBuy)
}

// uploadsInFlight lets an upload's verdict count the uploads to the same
// space started before it and not in the store yet: two uploads sent in
// parallel would otherwise both be told the same room is theirs.
type uploadsInFlight struct {
	mu      sync.Mutex
	seq     uint64
	bySpace map[string][]*inFlightUpload
}

type inFlightUpload struct {
	seq uint64
	// size is zero until known: a url upload learns it from its result
	size int64
	// id is empty until the upload returns
	id string
}

// start registers an upload of size bytes to the space; finish must follow.
func (u *uploadsInFlight) start(spaceId string, size int64) *inFlightUpload {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.seq++
	upload := &inFlightUpload{seq: u.seq, size: size}
	if u.bySpace == nil {
		u.bySpace = map[string][]*inFlightUpload{}
	}
	u.bySpace[spaceId] = append(u.bySpace[spaceId], upload)
	return upload
}

// resolve records what the upload stored.
func (u *uploadsInFlight) resolve(upload *inFlightUpload, id string, size int64) {
	u.mu.Lock()
	defer u.mu.Unlock()
	upload.id = id
	upload.size = size
}

func (u *uploadsInFlight) finish(spaceId string, upload *inFlightUpload) {
	u.mu.Lock()
	defer u.mu.Unlock()
	uploads := u.bySpace[spaceId]
	for i, other := range uploads {
		if other == upload {
			uploads = append(uploads[:i:i], uploads[i+1:]...)
			break
		}
	}
	if len(uploads) == 0 {
		delete(u.bySpace, spaceId)
		return
	}
	u.bySpace[spaceId] = uploads
}

// before lists the uploads to the space started before seq, oldest first.
func (u *uploadsInFlight) before(spaceId string, seq uint64) []*inFlightUpload {
	u.mu.Lock()
	defer u.mu.Unlock()
	var uploads []*inFlightUpload
	for _, upload := range u.bySpace[spaceId] {
		if upload.seq < seq {
			uploads = append(uploads, upload)
		}
	}
	return uploads
}

// resolved reads the uploads as they stand now: the ones of known size as
// files, and whether any size is still unknown.
func (u *uploadsInFlight) resolved(uploads []*inFlightUpload) (files []waitingFile, unknown bool) {
	u.mu.Lock()
	defer u.mu.Unlock()
	for _, upload := range uploads {
		if upload.size <= 0 {
			unknown = true
			continue
		}
		files = append(files, waitingFile{id: upload.id, size: upload.size})
	}
	return files, unknown
}

// stagedFileSize is the size of a staged upload, or false when it cannot be
// read.
func stagedFileSize(localPath string) (int64, bool) {
	if localPath == "" {
		return 0, false
	}
	info, err := os.Stat(localPath)
	if err != nil {
		return 0, false
	}
	return info.Size(), true
}

// syncNotes says in one sentence each what is wrong with the space's sync
// and what fixes it. Buying storage is advised only where upgrade_needed is
// true; a remedy is offered only for a shortage a current report shows.
func syncNotes(st *v2model.SyncStatus, checked bool, payer spacePayer, quota *quotaView, verdict storageVerdict, refused refusedFileCount) []string {
	var notes []string
	switch {
	case st.Network == v2model.SyncNetworkLocalOnly:
		return append(notes, "This account is in local-only mode: nothing is backed up to a node.")
	case st.Error == v2model.SyncErrorIncompatibleVersion:
		notes = append(notes, "This app version cannot sync with the network. Update the app.")
	case st.Status == v2model.SyncStatusNetworkNeedsUpdate:
		notes = append(notes, "The sync network needs a newer app version. Update the app.")
	case st.Error == v2model.SyncErrorNetwork:
		notes = append(notes, "No connection to the backup node. Changes sync once it is back.")
	case !checked:
		notes = append(notes, "This device has not reached the backup node yet, so the status is not confirmed.")
	}
	remedy := storageRemedy(st.Network, payer, st.UpgradeNeeded)
	// a stale report says what was, not what to do: the plan may have
	// changed since
	current := quota != nil && verdict.fresh
	ownRefused := refused.count - refused.othersCount
	if refused.count > 0 {
		note := fmt.Sprintf("%d files did not fit in the storage limit and are not backed up.", refused.count)
		if refused.count == 1 {
			note = "1 file did not fit in the storage limit and is not backed up."
		}
		if refused.othersCount > 0 {
			note += " In a one-to-one space each member's files count against their own storage."
		}
		switch {
		case !current || ownRefused == 0:
		case verdict.short:
			note += " " + remedy
		case verdict.roomForRefused && refused.othersCount > 0:
			note += " There is room now for yours: they upload from the device that added them."
		case verdict.roomForRefused:
			note += " There is room now: they upload from the device that added them."
		}
		notes = append(notes, note)
	}
	// the caller's own storage, when no refusal of theirs already spoke of it
	if ownRefused == 0 {
		switch {
		case quota == nil:
		case verdict.short && !current:
			notes = append(notes, "The storage limit was reached at the backup node's last report.")
		case verdict.short:
			notes = append(notes, "The storage limit is reached: new files will not be backed up. "+remedy)
		case verdict.waitingDoNotFit:
			notes = append(notes, "Files waiting to upload may not fit in the storage left. "+remedy)
		}
	}
	return notes
}

// storageRemedy says who can make room, and how. canBuy is the one case
// buying storage is advised: see canBuyStorage.
func storageRemedy(network string, payer spacePayer, canBuy bool) string {
	switch {
	case canBuy:
		return "Upgrade the plan or delete files to free space."
	case network == v2model.SyncNetworkSelfHosted && payer.kind == payerCaller:
		return "Delete files to free space, or ask the node's administrator to raise the limit."
	case network == v2model.SyncNetworkSelfHosted:
		return "Delete files you added to free space; the space owner or the node's administrator can make more room."
	case payer.kind == payerCaller:
		return "Delete files to free space."
	case payer.kind == payerOther:
		return "The limit is the space owner's: delete files you added to free space, or ask the owner for more room."
	default:
		return "Delete files to free space, or ask the space owner for more room."
	}
}

// syncStatusName maps the app's space status to its wire value. Three
// mappings differ from the app on purpose. A local-only account is
// local_only rather than offline: nothing is wrong with its connection. A
// connection not tried yet is connecting rather than synced: the app shows
// it as fine until the first check. And the app's storage warning
// (syncErrorName) does not make the status error.
func syncStatusName(event *pb.EventSpaceSyncStatusUpdate, checked bool) string {
	if event.Network == pb.EventSpace_LocalOnly {
		return v2model.SyncStatusLocalOnly
	}
	if event.Status == pb.EventSpace_Error && event.Error == pb.EventSpace_StorageLimitExceed {
		if event.SyncingObjectsCounter > 0 {
			return syncStatusUnchecked(v2model.SyncStatusSyncing, checked)
		}
		return syncStatusUnchecked(v2model.SyncStatusSynced, checked)
	}
	switch event.Status {
	case pb.EventSpace_Synced:
		return syncStatusUnchecked(v2model.SyncStatusSynced, checked)
	case pb.EventSpace_Syncing:
		return syncStatusUnchecked(v2model.SyncStatusSyncing, checked)
	case pb.EventSpace_Offline:
		return v2model.SyncStatusOffline
	case pb.EventSpace_NetworkNeedsUpdate:
		return v2model.SyncStatusNetworkNeedsUpdate
	default:
		return v2model.SyncStatusError
	}
}

func syncStatusUnchecked(status string, checked bool) string {
	if checked {
		return status
	}
	return v2model.SyncStatusConnecting
}

func syncNetworkName(network pb.EventSpaceNetwork) string {
	switch network {
	case pb.EventSpace_SelfHost:
		return v2model.SyncNetworkSelfHosted
	case pb.EventSpace_LocalOnly:
		return v2model.SyncNetworkLocalOnly
	default:
		return v2model.SyncNetworkAnytype
	}
}

// syncErrorName maps the app's space error to its wire value. The app's
// StorageLimitExceed is not served: it is raised below 10% of the CALLER's
// own account storage left, whichever space this is, and also when that
// usage could not be read. The storage block and upgrade_needed are
// computed from the quota that actually applies to the space.
func syncErrorName(err pb.EventSpaceSyncError) string {
	switch err {
	case pb.EventSpace_NetworkError:
		return v2model.SyncErrorNetwork
	case pb.EventSpace_IncompatibleVersion:
		return v2model.SyncErrorIncompatibleVersion
	default:
		return ""
	}
}

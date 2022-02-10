package subscription

func newListDiff(ids []string) *listDiff {
	ld := &listDiff{
		before: make(map[string]string),
		after:  make(map[string]string),
	}
	var prevId string
	for _, id := range ids {
		ld.before[id] = prevId
		prevId = id
	}
	return ld
}

type listDiff struct {
	// map[id]prevId
	before, after map[string]string
	afterIds      []string
}

func (ld *listDiff) fillAfter(id string) {
	ld.afterIds = append(ld.afterIds, id)
}

func (ld *listDiff) reverse() {
	for i, j := 0, len(ld.afterIds)-1; i < j; i, j = i+1, j-1 {
		ld.afterIds[i], ld.afterIds[j] = ld.afterIds[j], ld.afterIds[i]
	}
}

func (ld *listDiff) reset() {
	ld.before = ld.after
	ld.after = make(map[string]string)
	ld.afterIds = ld.afterIds[:0]
}

func (ld *listDiff) diff(ctx *opCtx, subId string, keys []string) {
	var ctxChangesLen = len(ctx.change)
	var _ = func(prevId, bPrevId string) bool {
		// by changes
		ctxChanges := ctx.change[ctxChangesLen:]
		for {
			found := false
			for _, ch := range ctxChanges {
				if ch.id == prevId {
					found = true
				}
			}
			if found {
				if prevId = ld.after[prevId]; prevId == "" {
					break
				}
			} else {
				break
			}
		}
		return prevId != bPrevId
	}
	var changeNeededAdd = func(prevId, bPrevId string) bool {
		// by add
		for {
			if _, ok := ld.before[prevId]; !ok {
				if prevId = ld.after[prevId]; prevId == "" {
					break
				}
			} else {
				break
			}
		}
		return prevId != bPrevId
	}
	var id, prevId, bPrevId string
	var ok bool
	for _, id = range ld.afterIds {
		if bPrevId, ok = ld.before[id]; ok {
			// change position
			if bPrevId != prevId && changeNeededAdd(prevId, bPrevId) {
				ctx.change = append(ctx.change, opChange{
					id:      id,
					subId:   subId,
					keys:    keys,
					afterId: prevId,
				})
			}
		} else {
			// add
			ctx.change = append(ctx.change, opChange{
				id:      id,
				subId:   subId,
				keys:    keys,
				afterId: prevId,
				isAdd:   true,
			})
		}
		ld.after[id] = prevId
		prevId = id
	}
	for id = range ld.before {
		if _, ok = ld.after[id]; !ok {
			ctx.remove = append(ctx.remove, opRemove{
				id:    id,
				subId: subId,
			})
		}
	}
}

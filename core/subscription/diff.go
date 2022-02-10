package subscription

type listDiff struct {
	beforeFirstId, afterFirstId string
	// map[id]prevId
	before, after   map[string]string
	fillAfterLastId string
}

func (ld *listDiff) fillAfter(id string) {
	ld.after[id] = ld.fillAfterLastId
	ld.fillAfterLastId = id
}

func (ld *listDiff) fillAfterReverse(id string) {
	if ld.fillAfterLastId != "" {
		ld.after[ld.fillAfterLastId] = id
	}
	ld.fillAfterLastId = id
}

func (ld *listDiff) fillAfterReverseFinalize() {
	ld.after[ld.fillAfterLastId] = ""
}

func (ld *listDiff) reset() {
	ld.before = ld.after
	ld.after = make(map[string]string)
	ld.beforeFirstId = ld.afterFirstId
	ld.afterFirstId = ""
	ld.fillAfterLastId = ""
}

func (ld *listDiff) diff(ctx *opCtx, subId string, keys []string) {
	var id, prevId, bPrevId string
	var ok bool
	for id, prevId = range ld.after {
		if bPrevId, ok = ld.before[id]; ok {
			// change position
			if bPrevId != prevId {
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

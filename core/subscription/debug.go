//go:build anydebug

package subscription

import (
	"fmt"
	"io"
	"net/http"
	"sort"
	"sync"

	"github.com/go-chi/chi/v5"
	"github.com/gogo/protobuf/jsonpb"

	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/util/debug"
)

func (s *service) DebugRouter(r chi.Router) {
	r.Get("/per_object/{spaceId}", debug.PlaintextHandler(s.debugPerObject))
	r.Get("/per_subscription/{spaceId}", debug.PlaintextHandler(s.debugPerSubscription))
}

func (s *service) debugPerObject(w io.Writer, req *http.Request) error {
	spaceId := chi.URLParam(req, "spaceId")
	spaceSub, ok := s.spaceSubs[spaceId]
	if !ok {
		return fmt.Errorf("no sub for space %s", spaceId)
	}
	return spaceSub.subDebugger.printPerObject(w, req)
}

func (s *service) debugPerSubscription(w io.Writer, req *http.Request) error {
	spaceId := chi.URLParam(req, "spaceId")
	spaceSub, ok := s.spaceSubs[spaceId]
	if !ok {
		return fmt.Errorf("no sub for space %s", spaceId)
	}
	return spaceSub.subDebugger.printPerSubscription(w, req)
}

func (s *spaceSubscriptions) initDebugger() {
	s.subDebugger = newSubDebugger()
}

func (s *spaceSubscriptions) debugEvents(ev *pb.Event) {
	for _, msg := range ev.Messages {
		s.subDebugger.addEvent(msg)
	}
}

type debugEntrySet pb.EventMessageValueOfObjectDetailsSet

func (e *debugEntrySet) getObjectId() string {
	return e.ObjectDetailsSet.Id
}

func (e *debugEntrySet) getSubscriptionIds() []string {
	return e.ObjectDetailsSet.SubIds
}

func (e *debugEntrySet) perObjectString() string {
	return fmt.Sprint("SET   ", e.getSubscriptionIds())
}

func (e *debugEntrySet) perSubscriptionString() string {
	return fmt.Sprint("SET   ", e.getObjectId())
}

func (e *debugEntrySet) describe(w io.Writer) error {
	marshaller := &jsonpb.Marshaler{}
	keys := make([]string, 0, len(e.ObjectDetailsSet.Details.GetFields()))
	for k := range e.ObjectDetailsSet.Details.GetFields() {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		value, err := marshaller.MarshalToString(e.ObjectDetailsSet.Details.GetFields()[k])
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "\t\t%s: %s\n", k, value)
	}
	return nil
}

type debugEntryUnset pb.EventMessageValueOfObjectDetailsUnset

func (e *debugEntryUnset) getObjectId() string {
	return e.ObjectDetailsUnset.Id
}

func (e *debugEntryUnset) getSubscriptionIds() []string {
	return e.ObjectDetailsUnset.SubIds
}

func (e *debugEntryUnset) perObjectString() string {
	return fmt.Sprint("UNSET ", e.getSubscriptionIds())
}

func (e *debugEntryUnset) perSubscriptionString() string {
	return fmt.Sprint("UNSET ", e.getObjectId())
}

func (e *debugEntryUnset) describe(w io.Writer) error {
	for _, k := range e.ObjectDetailsUnset.Keys {
		fmt.Fprintf(w, "\t\t%s\n", k)
	}
	return nil
}

type debugEntryAmend pb.EventMessageValueOfObjectDetailsAmend

func (e *debugEntryAmend) getObjectId() string {
	return e.ObjectDetailsAmend.Id
}

func (e *debugEntryAmend) getSubscriptionIds() []string {
	return e.ObjectDetailsAmend.SubIds
}

func (e *debugEntryAmend) perObjectString() string {
	return fmt.Sprint("AMEND ", e.getSubscriptionIds())
}

func (e *debugEntryAmend) perSubscriptionString() string {
	return fmt.Sprint("AMEND ", e.getObjectId())
}

func (e *debugEntryAmend) describe(w io.Writer) error {
	marshaller := &jsonpb.Marshaler{}

	converted := make([]keyValue, 0, len(e.ObjectDetailsAmend.Details))
	for _, det := range e.ObjectDetailsAmend.Details {
		value, err := marshaller.MarshalToString(det.Value)
		if err != nil {
			return err
		}
		converted = append(converted, keyValue{
			key:   det.Key,
			value: value,
		})
	}
	sort.Slice(converted, func(i, j int) bool {
		return converted[i].key < converted[j].key
	})
	for _, kv := range converted {
		fmt.Fprintf(w, "\t\t%s: %s\n", kv.key, kv.value)
	}
	return nil
}

type debugEntryAdd pb.EventMessageValueOfSubscriptionAdd

func (e *debugEntryAdd) getObjectId() string {
	return e.SubscriptionAdd.Id
}

func (e *debugEntryAdd) getSubscriptionIds() []string {
	return []string{e.SubscriptionAdd.SubId}
}

func (e *debugEntryAdd) perObjectString() string {
	return fmt.Sprintf("[ADD] %s --- %q", e.getSubscriptionIds(), e.SubscriptionAdd.AfterId)
}

func (e *debugEntryAdd) perSubscriptionString() string {
	return fmt.Sprintf("[ADD] %s --- %q", e.getObjectId(), e.SubscriptionAdd.AfterId)
}

func (e *debugEntryAdd) describe(w io.Writer) error {
	return nil
}

type debugEntryRemove pb.EventMessageValueOfSubscriptionRemove

func (e *debugEntryRemove) getObjectId() string {
	return e.SubscriptionRemove.Id
}

func (e *debugEntryRemove) getSubscriptionIds() []string {
	return []string{e.SubscriptionRemove.SubId}
}

func (e *debugEntryRemove) perObjectString() string {
	return fmt.Sprint("[REM] ", e.getSubscriptionIds())
}

func (e *debugEntryRemove) perSubscriptionString() string {
	return fmt.Sprintf("[REM] %s", e.getObjectId())
}

func (e *debugEntryRemove) describe(w io.Writer) error {
	return nil
}

type debugEntryPosition pb.EventMessageValueOfSubscriptionPosition

func (e *debugEntryPosition) getObjectId() string {
	return e.SubscriptionPosition.Id
}

func (e *debugEntryPosition) getSubscriptionIds() []string {
	return []string{e.SubscriptionPosition.SubId}
}

func (e *debugEntryPosition) perObjectString() string {
	return fmt.Sprintf("[POS] %s --- %q", e.getSubscriptionIds(), e.SubscriptionPosition.AfterId)
}

func (e *debugEntryPosition) perSubscriptionString() string {
	return fmt.Sprintf("[POS] %s --- %q", e.getObjectId(), e.SubscriptionPosition.AfterId)
}

func (e *debugEntryPosition) describe(w io.Writer) error {
	return nil
}

type debugEntryCounters pb.EventMessageValueOfSubscriptionCounters

func (e *debugEntryCounters) getObjectId() string {
	return ""
}

func (e *debugEntryCounters) getSubscriptionIds() []string {
	return []string{e.SubscriptionCounters.SubId}
}

func (e *debugEntryCounters) perObjectString() string {
	return ""
}

func (e *debugEntryCounters) perSubscriptionString() string {
	return fmt.Sprintf("[CNT] total=%d", e.SubscriptionCounters.Total)
}

func (e *debugEntryCounters) describe(w io.Writer) error {
	fmt.Fprintf(w, "\t\tprev=%d next=%d total=%d\n", e.SubscriptionCounters.PrevCount, e.SubscriptionCounters.NextCount, e.SubscriptionCounters.Total)
	return nil
}

func (e *debugEntryCounters) noObjectInfo() {}

type debugGroups pb.EventMessageValueOfSubscriptionGroups

func (e *debugGroups) getObjectId() string {
	return ""
}

func (e *debugGroups) getSubscriptionIds() []string {
	return []string{e.SubscriptionGroups.SubId}
}

func (e *debugGroups) perObjectString() string {
	return ""
}

func (e *debugGroups) perSubscriptionString() string {
	var removeStr string
	if e.SubscriptionGroups.Remove {
		removeStr = "REMOVE "
	}
	return fmt.Sprintf("[GRP] %s%s", removeStr, e.SubscriptionGroups.Group)
}

func (e *debugGroups) describe(w io.Writer) error {
	return nil
}

type withoutObjectInfo interface {
	noObjectInfo()
}

type debugEntry interface {
	getObjectId() string
	getSubscriptionIds() []string
	perObjectString() string
	perSubscriptionString() string
	describe(w io.Writer) error
}

func newDebugEntry(msg *pb.EventMessage) debugEntry {
	switch v := msg.Value.(type) {
	case *pb.EventMessageValueOfObjectDetailsSet:
		return (*debugEntrySet)(v)
	case *pb.EventMessageValueOfObjectDetailsUnset:
		return (*debugEntryUnset)(v)
	case *pb.EventMessageValueOfObjectDetailsAmend:
		return (*debugEntryAmend)(v)
	case *pb.EventMessageValueOfSubscriptionAdd:
		return (*debugEntryAdd)(v)
	case *pb.EventMessageValueOfSubscriptionRemove:
		return (*debugEntryRemove)(v)
	case *pb.EventMessageValueOfSubscriptionPosition:
		return (*debugEntryPosition)(v)
	case *pb.EventMessageValueOfSubscriptionCounters:
		return (*debugEntryCounters)(v)
	case *pb.EventMessageValueOfSubscriptionGroups:
		return (*debugGroups)(v)
	default:
		return nil
	}
}

type keyValue struct {
	key   string
	value string
}

type subDebugger struct {
	lock            sync.RWMutex
	eventsPerObject map[string][]debugEntry
	objectNames     map[string]string
	objectIds       []string

	eventsPerSubscription map[string][]debugEntry
	subscriptionIds       []string
}

func newSubDebugger() *subDebugger {
	return &subDebugger{
		eventsPerObject:       map[string][]debugEntry{},
		eventsPerSubscription: map[string][]debugEntry{},
		objectNames:           map[string]string{},
	}
}

func (d *subDebugger) addEvent(msg *pb.EventMessage) {
	var name string
	if v := msg.GetObjectDetailsSet(); v != nil {
		name = v.Details.Fields[bundle.RelationKeyName.String()].GetStringValue()
	}

	ent := newDebugEntry(msg)
	objectId := ent.getObjectId()
	subscriptionIds := ent.getSubscriptionIds()

	d.lock.Lock()
	defer d.lock.Unlock()

	if name != "" {
		d.objectNames[objectId] = name
	}

	if _, ok := ent.(withoutObjectInfo); !ok {
		if _, ok := d.eventsPerObject[objectId]; !ok {
			d.objectIds = append(d.objectIds, objectId)
		}
		d.eventsPerObject[objectId] = append(d.eventsPerObject[objectId], ent)
	}

	for _, subId := range subscriptionIds {
		if _, ok := d.eventsPerSubscription[subId]; !ok {
			d.subscriptionIds = append(d.subscriptionIds, subId)
		}
		d.eventsPerSubscription[subId] = append(d.eventsPerSubscription[subId], ent)
	}
}

func (d *subDebugger) printPerObject(w io.Writer, _ *http.Request) error {
	d.lock.RLock()
	defer d.lock.RUnlock()
	for _, id := range d.objectIds {
		fmt.Fprintf(w, "%s %s\n", id, d.objectNames[id])
		for _, e := range d.eventsPerObject[id] {
			fmt.Fprintf(w, "\t%s\n", e.perObjectString())
			err := e.describe(w)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (d *subDebugger) printPerSubscription(w io.Writer, _ *http.Request) error {
	d.lock.RLock()
	defer d.lock.RUnlock()
	for _, subId := range d.subscriptionIds {
		fmt.Fprintf(w, "%s\n", subId)
		for _, entry := range d.eventsPerSubscription[subId] {
			fmt.Fprintf(w, "\t%s\n", entry.perSubscriptionString())
			err := entry.describe(w)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

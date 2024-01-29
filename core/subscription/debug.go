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
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func (s *service) DebugRouter(r chi.Router) {
	r.Get("/per_object", debug.PlaintextHandler(s.debugPerObject))
	r.Get("/per_subscription", debug.PlaintextHandler(s.debugPerSubscription))
}

func (s *service) debugPerObject(w io.Writer, req *http.Request) error {
	return s.subDebugger.printPerObject(w, req)
}

func (s *service) debugPerSubscription(w io.Writer, req *http.Request) error {
	return s.subDebugger.printPerSubscription(w, req)
}

type debugEntry struct {
	rawMessage *pb.EventMessage
}

func (e debugEntry) getObjectId() string {
	switch v := e.rawMessage.Value.(type) {
	case *pb.EventMessageValueOfObjectDetailsSet:
		return v.ObjectDetailsSet.Id
	case *pb.EventMessageValueOfObjectDetailsUnset:
		return v.ObjectDetailsUnset.Id
	case *pb.EventMessageValueOfObjectDetailsAmend:
		return v.ObjectDetailsAmend.Id
	case *pb.EventMessageValueOfSubscriptionAdd:
		return v.SubscriptionAdd.Id
	case *pb.EventMessageValueOfSubscriptionRemove:
		return v.SubscriptionRemove.Id
	case *pb.EventMessageValueOfSubscriptionPosition:
		return v.SubscriptionPosition.Id
	default:
		return fmt.Sprintf("UNKNOWN %T", e.rawMessage.Value)
	}
}

func (e debugEntry) getSubscriptionIds() []string {
	switch v := e.rawMessage.Value.(type) {
	case *pb.EventMessageValueOfObjectDetailsSet:
		return v.ObjectDetailsSet.SubIds
	case *pb.EventMessageValueOfObjectDetailsUnset:
		return v.ObjectDetailsUnset.SubIds
	case *pb.EventMessageValueOfObjectDetailsAmend:
		return v.ObjectDetailsAmend.SubIds
	case *pb.EventMessageValueOfSubscriptionAdd:
		return []string{v.SubscriptionAdd.SubId}
	case *pb.EventMessageValueOfSubscriptionRemove:
		return []string{v.SubscriptionRemove.SubId}
	case *pb.EventMessageValueOfSubscriptionPosition:
		return []string{v.SubscriptionPosition.SubId}
	default:
		return nil
	}
}

func (e debugEntry) perObjectString() string {
	switch v := e.rawMessage.Value.(type) {
	case *pb.EventMessageValueOfObjectDetailsSet:
		return fmt.Sprint("SET   ", e.getSubscriptionIds())
	case *pb.EventMessageValueOfObjectDetailsUnset:
		return fmt.Sprint("UNSET ", e.getSubscriptionIds())
	case *pb.EventMessageValueOfObjectDetailsAmend:
		return fmt.Sprint("AMEND ", e.getSubscriptionIds())
	case *pb.EventMessageValueOfSubscriptionAdd:
		return fmt.Sprintf("[ADD] %s --- %q", e.getSubscriptionIds(), v.SubscriptionAdd.AfterId)
	case *pb.EventMessageValueOfSubscriptionRemove:
		return fmt.Sprint("[REM] ", e.getSubscriptionIds())
	case *pb.EventMessageValueOfSubscriptionPosition:
		return fmt.Sprintf("[POS] %s --- %q", e.getSubscriptionIds(), v.SubscriptionPosition.AfterId)
	default:
		return fmt.Sprintf("UNKNOWN %T", e.rawMessage.Value)
	}
}

func (e debugEntry) perSubscriptionString() string {
	switch v := e.rawMessage.Value.(type) {
	case *pb.EventMessageValueOfObjectDetailsSet:
		return fmt.Sprint("SET   ", e.getObjectId())
	case *pb.EventMessageValueOfObjectDetailsUnset:
		return fmt.Sprint("UNSET ", e.getObjectId())
	case *pb.EventMessageValueOfObjectDetailsAmend:
		return fmt.Sprint("AMEND ", e.getObjectId())
	case *pb.EventMessageValueOfSubscriptionAdd:
		return fmt.Sprintf("[ADD] %s --- %q", e.getObjectId(), v.SubscriptionAdd.AfterId)
	case *pb.EventMessageValueOfSubscriptionRemove:
		return fmt.Sprint("[REM] ", v.SubscriptionRemove.SubId)
	case *pb.EventMessageValueOfSubscriptionPosition:
		return fmt.Sprintf("[POS] %s --- %q", e.getObjectId(), v.SubscriptionPosition.AfterId)
	default:
		return fmt.Sprintf("UNKNOWN %T", e.rawMessage.Value)
	}
}

type keyValue struct {
	key   string
	value string
}

func (e debugEntry) printDetails(w io.Writer) error {
	switch v := e.rawMessage.Value.(type) {
	case *pb.EventMessageValueOfObjectDetailsSet:
		marshaller := &jsonpb.Marshaler{}
		keys := make([]string, 0, len(v.ObjectDetailsSet.Details.GetFields()))
		for k := range v.ObjectDetailsSet.Details.GetFields() {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			value, err := marshaller.MarshalToString(v.ObjectDetailsSet.Details.GetFields()[k])
			if err != nil {
				return err
			}
			fmt.Fprintf(w, "\t\t%s: %s\n", k, value)
		}
	case *pb.EventMessageValueOfObjectDetailsUnset:
		keys := make([]string, len(v.ObjectDetailsUnset.Keys))
		copy(keys, v.ObjectDetailsUnset.Keys)
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(w, "\t\t%s\n", k)
		}
	case *pb.EventMessageValueOfObjectDetailsAmend:
		marshaller := &jsonpb.Marshaler{}

		converted := make([]keyValue, 0, len(v.ObjectDetailsAmend.Details))
		for _, det := range v.ObjectDetailsAmend.Details {
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
	case *pb.EventMessageValueOfSubscriptionAdd:
	case *pb.EventMessageValueOfSubscriptionRemove:
	case *pb.EventMessageValueOfSubscriptionPosition:
	default:
		return fmt.Errorf("UNKNOWN %T", e.rawMessage.Value)
	}
	return nil
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
		name = pbtypes.GetString(v.Details, bundle.RelationKeyName.String())
	}

	ev := debugEntry{
		rawMessage: msg,
	}
	objectId := ev.getObjectId()
	subscriptionIds := ev.getSubscriptionIds()

	d.lock.Lock()
	defer d.lock.Unlock()

	if name != "" {
		d.objectNames[objectId] = name
	}

	if _, ok := d.eventsPerObject[objectId]; !ok {
		d.objectIds = append(d.objectIds, objectId)
	}
	d.eventsPerObject[objectId] = append(d.eventsPerObject[objectId], debugEntry{
		rawMessage: msg,
	})

	for _, subId := range subscriptionIds {
		if _, ok := d.eventsPerSubscription[subId]; !ok {
			d.subscriptionIds = append(d.subscriptionIds, subId)
		}
		d.eventsPerSubscription[subId] = append(d.eventsPerSubscription[subId], debugEntry{
			rawMessage: msg,
		})
	}
}

func (d *subDebugger) printPerObject(w io.Writer, _ *http.Request) error {
	d.lock.RLock()
	defer d.lock.RUnlock()
	for _, id := range d.objectIds {
		fmt.Fprintf(w, "%s %s\n", id, d.objectNames[id])
		for _, entry := range d.eventsPerObject[id] {
			fmt.Fprintf(w, "\t%s\n", entry.perObjectString())
			entry.printDetails(w)
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
			entry.printDetails(w)
		}
	}
	return nil
}

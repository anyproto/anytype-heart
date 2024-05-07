package spacesyncstatus

type ObjectState struct {
	objectSyncInProgressBySpace map[string]bool
	objectSyncCountBySpace      map[string]int
}

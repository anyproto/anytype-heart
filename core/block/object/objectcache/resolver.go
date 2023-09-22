package objectcache

type resolver struct {
}

func (r *resolver) ResolveSpaceID(objectID string) (spaceID string, err error) {
	return "", err
}

func (r *resolver) StoreSpaceID(spaceID, objectID string) (err error) {
	return err
}

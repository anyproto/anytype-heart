package objectcache

type resolver struct {
}

func (r *resolver) ResolveSpaceID(objectID string) (spaceID string, err error) {
	return "", err
}

func (r *resolver) StoreSpaceID(objectID string, spaceID string) (err error) {
	return err
}

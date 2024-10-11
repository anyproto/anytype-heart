package domain

type BundledObjectId struct {
	SourceId        string
	DerivedObjectId string
}

type BundledObjectIds []BundledObjectId

func (b BundledObjectIds) Len() int {
	return len(b)
}

func (b BundledObjectIds) SourceIds() []string {
	ids := make([]string, 0, len(b))
	for _, bo := range b {
		ids = append(ids, bo.SourceId)
	}
	return ids
}

func (b BundledObjectIds) DerivedObjectIds() []string {
	ids := make([]string, 0, len(b))
	for _, bo := range b {
		ids = append(ids, bo.DerivedObjectId)
	}
	return ids
}

func (b BundledObjectIds) Filter(f func(bo BundledObjectId) bool) BundledObjectIds {
	var res = make([]BundledObjectId, 0, len(b))
	for _, bo := range b {
		if f(bo) {
			res = append(res, bo)
		}
	}
	return res
}

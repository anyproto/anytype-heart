package sourceimpl

// Thin exported aliases so the chatobject scenario harness (different package)
// can drive the exact production watermark predicate as a sign-off gate.
// Not used by production code.

type Watermark = watermark
type ReadPair = readPair

func NewWatermark(onRemove func([]string)) *Watermark { return newWatermark(onRemove) }

func (w *Watermark) Advance(seen []string, resolve func(string) (ReadPair, bool), each func(func(string, ReadPair))) {
	w.advance(seen, resolve, each)
}

func (w *Watermark) SeenHeadIds() []string { return w.seenHeadIds() }

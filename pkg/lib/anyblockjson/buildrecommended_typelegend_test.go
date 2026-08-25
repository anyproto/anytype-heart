package anyblockjson

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The missing half of TestBuildRecommendedLists_HonoursTheLegendItIsHandedOver:
// object_types is a TYPE key slot and it runs through the SAME legend, but no
// test names it, so opts.legendTypeKey can be reverted to opts.typeKey and the
// whole package stays green while a type's property targets re-point.
type recTypeVocab struct {
	BundledKeyVocabulary
	slugs map[string]string
}

func (v recTypeVocab) TypeSlug(k string) string {
	if s, ok := v.slugs[k]; ok {
		return s
	}
	return BundledKeyVocabulary{}.TypeSlug(k)
}
func (v recTypeVocab) TypeKey(s string) (string, bool) {
	for k, sl := range v.slugs {
		if sl == s {
			return k, true
		}
	}
	return BundledKeyVocabulary{}.TypeKey(s)
}

type capturingResolver struct{ seen *[]string }

func (r capturingResolver) PropertyById(string) (PropertyDefinition, bool) {
	return PropertyDefinition{}, false
}
func (r capturingResolver) PropertyId(def PropertyDefinition) (string, bool) {
	*r.seen = append(*r.seen, def.ObjectTypes...)
	return "id-" + string(def.Key), true
}

func TestBuildRecommendedLists_ObjectTypesHonourTheLegendToo(t *testing.T) {
	const liveType = "69bbfc78877a91b1d12d1a7c"
	props := []TypeProperty{{Property: "who", Section: "featured",
		Format: "objects", ObjectTypes: []string{"initiative"}}}

	// the decoy: this reader binds the spelling `initiative` to another type
	base := Options{Keys: recTypeVocab{slugs: map[string]string{"decoyType": "initiative"}}}

	t.Run("without the legend the reader's own answer wins", func(t *testing.T) {
		var seen []string
		o := base
		o.ResolveProperties = capturingResolver{seen: &seen}
		_, err := BuildRecommendedLists(props, o)
		require.NoError(t, err)
		assert.Equal(t, []string{"decoyType"}, seen)
	})

	t.Run("with it the document's own statement is chain step 1", func(t *testing.T) {
		var seen []string
		o := base
		o.ResolveProperties = capturingResolver{seen: &seen}
		o.Legend = Legend{TypeKeys: map[string]string{"initiative": liveType}}
		_, err := BuildRecommendedLists(props, o)
		require.NoError(t, err)
		assert.Equal(t, []string{liveType}, seen,
			"the property's targets name the types the document meant")
	})
}

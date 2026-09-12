package planfixture_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/importv2/schemaplan"
	"github.com/anyproto/anytype-heart/core/block/importv2/schemaplan/planfixture"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestLoad(t *testing.T) {
	t.Run("string format names become relation formats", func(t *testing.T) {
		// given
		want := map[string]model.RelationFormat{
			"pSku":         model.RelationFormat_longtext,
			"pPrice":       model.RelationFormat_number,
			"pStatus":      model.RelationFormat_status,
			"pYarnUsed":    model.RelationFormat_tag,
			"pPhotos":      model.RelationFormat_file,
			"pListingUrl":  model.RelationFormat_url,
			"pDateListed":  model.RelationFormat_date,
			"pMadeToOrder": model.RelationFormat_checkbox,
		}

		// when
		fixture, err := planfixture.Load("mixed-etsy-maker-shop")

		// then
		require.NoError(t, err)
		listings := containerById(t, fixture, "db-listings")
		got := map[string]model.RelationFormat{}
		for _, property := range listings.Properties {
			if _, ok := want[property.Id]; ok {
				got[property.Id] = property.Format
			}
		}
		assert.Equal(t, want, got)
	})

	t.Run("select options and sample titles survive the load", func(t *testing.T) {
		// when
		fixture, err := planfixture.Load("mixed-etsy-maker-shop")

		// then
		require.NoError(t, err)
		orders := containerById(t, fixture, "db-orders")
		status := propertyById(t, orders, "pStatus")
		assert.Equal(t, []string{"New", "Making", "Shipped", "Delivered", "Refunded"}, status.Options)
		require.NotNil(t, orders.Samples)
		assert.Len(t, orders.Samples.Titles, 5)
	})

	t.Run("expectations load with bundled targets as relation keys", func(t *testing.T) {
		// when
		fixture, err := planfixture.Load("mixed-etsy-maker-shop")

		// then
		require.NoError(t, err)
		assert.Equal(t, model.RelationFormat_email,
			propertyById(t, containerById(t, fixture, "db-customers"), "pEmail").Format)
		assert.Equal(t, "email", string(fixture.Expect.Bundled["db-customers"]["pEmail"]))
		assert.Contains(t, fixture.Expect.NotBundled["db-orders"], "pShipBy")
		assert.Contains(t, fixture.Expect.SameKind,
			[]string{"db-shipping-holiday", "db-shipping-summer"})
	})

	t.Run("unknown fixture name is an error, not a panic", func(t *testing.T) {
		// when
		_, err := planfixture.Load("no-such-fixture")

		// then
		require.Error(t, err)
	})
}

func TestAll(t *testing.T) {
	t.Run("every embedded fixture loads and is internally consistent", func(t *testing.T) {
		// when
		fixtures, err := planfixture.All()

		// then
		require.NoError(t, err)
		require.Len(t, fixtures, 5)

		totalContainers := 0
		for _, fixture := range fixtures {
			assert.NotEmpty(t, fixture.Id, "fixture id")
			assert.NotEmpty(t, fixture.Containers, "%s: containers", fixture.Id)
			totalContainers += len(fixture.Containers)

			ids := map[string]bool{}
			for _, container := range fixture.Containers {
				assert.False(t, ids[container.Id], "%s: duplicate container id %s", fixture.Id, container.Id)
				ids[container.Id] = true
				for _, property := range container.Properties {
					assert.NotEqual(t, model.RelationFormat(-1), property.Format,
						"%s.%s.%s: unmapped format", fixture.Id, container.Id, property.Id)
				}
			}
			// every id named by an expectation must exist
			for _, group := range append(append([][]string{}, fixture.Expect.SameKind...),
				fixture.Expect.DifferentKind...) {
				for _, containerId := range group {
					assert.True(t, ids[containerId],
						"%s: expectation names unknown container %s", fixture.Id, containerId)
				}
			}
		}
		assert.Equal(t, 61, totalContainers, "suite size changed — update the spec's §8 count")
	})
}

func containerById(t *testing.T, fixture planfixture.Fixture, id string) schemaplan.ContainerSchema {
	t.Helper()
	for _, container := range fixture.Containers {
		if container.Id == id {
			return container
		}
	}
	t.Fatalf("container %q not found in fixture %q", id, fixture.Id)
	return schemaplan.ContainerSchema{}
}

func propertyById(t *testing.T, container schemaplan.ContainerSchema, id string) schemaplan.PropertySchema {
	t.Helper()
	for _, property := range container.Properties {
		if property.Id == id {
			return property
		}
	}
	t.Fatalf("property %q not found in container %q", id, container.Id)
	return schemaplan.PropertySchema{}
}

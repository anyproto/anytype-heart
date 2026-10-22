package llmplan_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/ai/llmclient"
	importv2 "github.com/anyproto/anytype-heart/core/block/importv2"
	"github.com/anyproto/anytype-heart/core/block/importv2/llmplan"
	"github.com/anyproto/anytype-heart/core/block/importv2/schemaplan"
	"github.com/anyproto/anytype-heart/core/block/importv2/schemaplan/planfixture"
)

// TestLiveNamingQuality scores the kinds call on naming, over two corpora at
// once: the synthetic fixture suite and — when pointed at one — a dumped real
// workspace. It exists because the end-to-end report only surfaces a type's
// singular name, so the sharpest plural metric (does the model return the same
// string for name_singular and name_plural?) was not observable there. The
// regex "ends in s" proxy used before conflates a genuine number error with the
// model echoing a plural source label.
//
//	IMPORTV2_LLM_ENDPOINT=http://host:11434/v1 IMPORTV2_LLM_MODEL=gemma4:e2b \
//	IMPORTV2_REAL_SCHEMAS=/path/to/schemas.json \
//	  go test ./core/block/importv2/llmplan/ -run TestLiveNamingQuality -v -count=1 -timeout 40m
func TestLiveNamingQuality(t *testing.T) {
	endpoint := os.Getenv("IMPORTV2_LLM_ENDPOINT")
	key := os.Getenv("OPENAI_API_KEY")
	if endpoint == "" {
		endpoint = "https://api.openai.com/v1"
		if key == "" {
			t.Skip("set IMPORTV2_LLM_ENDPOINT or OPENAI_API_KEY")
		}
	}
	if key == "" {
		key = "local"
	}
	model := os.Getenv("IMPORTV2_LLM_MODEL")
	if model == "" {
		model = "gpt-4o-mini"
	}

	client, err := llmclient.New(llmclient.Config{Endpoint: endpoint, Model: model, Token: key})
	require.NoError(t, err)
	opts := []llmplan.Option{llmplan.WithBudget(20 * time.Minute)}
	if raw := os.Getenv("IMPORTV2_LLM_CHUNK"); raw != "" {
		size, err := strconv.Atoi(raw)
		require.NoError(t, err, "IMPORTV2_LLM_CHUNK must be an integer")
		opts = append(opts, llmplan.WithChunkSize(size))
		t.Logf("chunk size: %d", size)
		if rawC := os.Getenv("IMPORTV2_LLM_CHUNK_CONCURRENCY"); rawC != "" {
			n, err := strconv.Atoi(rawC)
			require.NoError(t, err)
			opts = append(opts, llmplan.WithChunkConcurrency(n))
			t.Logf("chunk concurrency: %d", n)
		}
	}
	// On ollama only "none" changes anything — it strips the <|think|> marker
	// from the system turn; "low" and "high" are byte-identical requests there.
	if effort := os.Getenv("IMPORTV2_LLM_EFFORT"); effort != "" {
		opts = append(opts, llmplan.WithReasoningEffort(effort))
		t.Logf("reasoning effort: %q", effort)
	}
	planner := llmplan.New(client, opts...)

	type corpus struct {
		name    string
		schemas []schemaplan.ContainerSchema
		// expect is set for synthetic fixtures: their sameKind/differentKind
		// assertions are pure LLM grouping decisions, and nothing else in the
		// suite checks a real model against them (the whitelist tests use
		// scripted plans).
		expect *planfixture.Expectations
	}
	var corpora []corpus

	// The synthetic fixtures are 10-14 containers, so they exercise naming but
	// not the size-dependent coverage gap; skip them when sweeping that.
	if os.Getenv("IMPORTV2_SKIP_FIXTURES") == "" {
		fixtures, err := planfixture.All()
		require.NoError(t, err)
		for _, fixture := range fixtures {
			expect := fixture.Expect
			corpora = append(corpora, corpus{"synthetic/" + fixture.Id, fixture.Containers, &expect})
		}
	}
	// A dumped []schemaplan.ContainerSchema — the real workspace's evidence.
	if path := os.Getenv("IMPORTV2_REAL_SCHEMAS"); path != "" {
		raw, err := os.ReadFile(path)
		require.NoError(t, err)
		var real []schemaplan.ContainerSchema
		require.NoError(t, json.Unmarshal(raw, &real))
		corpora = append(corpora, corpus{name: fmt.Sprintf("REAL/%d-containers", len(real)), schemas: real})
	}

	t.Logf("%-34s %8s %6s %9s %9s %9s %7s %8s %6s", "corpus", "assigned", "kinds", "sing==pl", "no plural", "echoed", "dupes", "grouping", "sec")
	for _, c := range corpora {
		started := time.Now()
		plan, err := planner.Plan(context.Background(), c.schemas)
		took := time.Since(started)
		if err != nil {
			t.Logf("%-34s FAILED: %v", c.name, err)
			continue
		}
		s := scoreNaming(plan, c.schemas)
		grouping := "-"
		if c.expect != nil {
			ok, total, misses := scoreGrouping(plan, *c.expect)
			grouping = fmt.Sprintf("%d/%d", ok, total)
			for _, miss := range misses {
				t.Logf("    %s GROUPING MISS: %s", c.name, miss)
			}
		}
		t.Logf("%-34s %5d/%-2d %6d %9s %9s %9s %7d %8s %6.0f  %s",
			c.name, s.assigned, len(c.schemas), s.kinds,
			fmt.Sprintf("%d/%d", s.sameSingularPlural, s.kinds),
			fmt.Sprintf("%d/%d", s.emptyPlural, s.kinds),
			fmt.Sprintf("%d/%d", s.echoed, s.kinds),
			s.duplicateNames, grouping, took.Seconds(), strings.Join(s.sample, ", "))
		if os.Getenv("IMPORTV2_DUMP_NAMES") != "" {
			// The counts score the model's raw answer; the import ships what
			// Sanitize makes of it. Print both, so a bad pair reads either as
			// "the model missed it" or as "the repair missed it".
			repaired := schemaplan.Sanitize(plan, c.schemas, func(importv2.Issue) {})
			after := make(map[string]schemaplan.TypeDefinition, len(repaired.NewTypes))
			for _, def := range repaired.NewTypes {
				after[string(def.Key)] = def
			}
			for _, def := range plan.NewTypes {
				fixed := after[string(def.Key)]
				t.Logf("    %-26s %-26s -> %-26s %s", def.Name, def.PluralName, fixed.Name, fixed.PluralName)
			}
		}
	}
}

// scoreGrouping checks the fixture's sameKind/differentKind assertions against
// what the model actually grouped. These are the assertions only a real model
// can satisfy — every other expectation in the suite is decided by code.
func scoreGrouping(plan schemaplan.Plan, expect planfixture.Expectations) (ok, total int, misses []string) {
	typeOf := func(containerId string) string {
		return string(plan.Containers[containerId].TypeKey)
	}
	for _, group := range expect.SameKind {
		total++
		want := typeOf(group[0])
		shared := want != ""
		for _, containerId := range group[1:] {
			if typeOf(containerId) != want {
				shared = false
			}
		}
		if shared {
			ok++
			continue
		}
		var got []string
		for _, containerId := range group {
			got = append(got, containerId+"="+typeOf(containerId))
		}
		misses = append(misses, "sameKind not shared: "+strings.Join(got, " "))
	}
	for _, group := range expect.DifferentKind {
		total++
		clash := ""
		for i := 0; i < len(group); i++ {
			for j := i + 1; j < len(group); j++ {
				if key := typeOf(group[i]); key != "" && key == typeOf(group[j]) {
					clash = group[i] + " and " + group[j] + " both " + key
				}
			}
		}
		if clash == "" {
			ok++
			continue
		}
		misses = append(misses, "differentKind merged: "+clash)
	}
	return ok, total, misses
}

type namingScore struct {
	assigned, kinds, sameSingularPlural, emptyPlural, echoed, duplicateNames int
	sample                                                                   []string
}

var nameNoise = regexp.MustCompile(`(?i)\s*\((SB|A1|\d+)\)|\s*\b(DB|Database)\b|[^\w\s&]`)

func normalizeName(s string) string {
	return strings.TrimSpace(strings.Join(strings.Fields(nameNoise.ReplaceAllString(s, " ")), " "))
}

func scoreNaming(plan schemaplan.Plan, schemas []schemaplan.ContainerSchema) namingScore {
	nameById := make(map[string]string, len(schemas))
	for _, schema := range schemas {
		nameById[schema.Id] = schema.Name
	}
	membersByType := map[string][]string{}
	assigned := map[string]bool{}
	for containerId, containerPlan := range plan.Containers {
		if containerPlan.TypeKey == "" {
			continue
		}
		assigned[containerId] = true
		key := string(containerPlan.TypeKey)
		membersByType[key] = append(membersByType[key], nameById[containerId])
	}

	score := namingScore{assigned: len(assigned), kinds: len(plan.NewTypes)}
	seen := map[string]int{}
	for _, def := range plan.NewTypes {
		seen[def.Name]++
		// A MISSING plural scored as fine here — EqualFold("Tasks", "") is
		// false — which is how a run where the model answered name_plural
		// with "" for most kinds passed this test and shipped types whose
		// plural field the user found empty.
		if strings.TrimSpace(def.PluralName) == "" {
			score.emptyPlural++
		} else if strings.EqualFold(strings.TrimSpace(def.Name), strings.TrimSpace(def.PluralName)) {
			score.sameSingularPlural++
		}
		for _, member := range membersByType[string(def.Key)] {
			if strings.EqualFold(normalizeName(def.Name), normalizeName(member)) {
				score.echoed++
				break
			}
		}
	}
	for _, count := range seen {
		if count > 1 {
			score.duplicateNames++
		}
	}
	names := make([]string, 0, len(plan.NewTypes))
	for _, def := range plan.NewTypes {
		names = append(names, def.Name)
	}
	sort.Strings(names)
	if len(names) > 5 {
		names = names[:5]
	}
	score.sample = names
	return score
}

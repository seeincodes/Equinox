package equivalence

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"strings"
	"time"

	"equinox/models"
)

// Config holds thresholds for equivalence detection.
type Config struct {
	EmbeddingSimilarityHigh float64
	EmbeddingSimilarityLow  float64
	JaccardThreshold        float64
	LevenshteinThreshold    float64
	ResolutionWindowHours   int
}

// Engine performs equivalence detection across canonical markets.
type Engine struct {
	cfg      Config
	embedder *EmbeddingClient
}

func NewEngine(cfg Config, embedder *EmbeddingClient) *Engine {
	return &Engine{cfg: cfg, embedder: embedder}
}

// CandidatePair is a pair of markets that passed Stage 1.
type CandidatePair struct {
	A       models.CanonicalMarket
	B       models.CanonicalMarket
	Jaccard float64
	Levenshtein float64
	Flags   []models.MatchFlag
}

// DetectGroups runs the full equivalence detection pipeline.
func (e *Engine) DetectGroups(ctx context.Context, markets []models.CanonicalMarket) ([]models.EquivalenceGroup, error) {
	candidates := e.stage1Filter(markets)

	slog.Info("stage 1 complete",
		"total_markets", len(markets),
		"candidate_pairs", len(candidates),
	)

	if len(candidates) == 0 {
		return nil, nil
	}

	groups, err := e.stage2Classify(ctx, candidates)
	if err != nil {
		return nil, fmt.Errorf("stage 2: %w", err)
	}

	for i := range groups {
		checkSettlementDivergence(&groups[i])
	}

	slog.Info("equivalence detection complete",
		"groups_formed", len(groups),
	)

	return groups, nil
}

// stage1Filter applies rule-based pre-filtering on all O(n²) pairs.
func (e *Engine) stage1Filter(markets []models.CanonicalMarket) []CandidatePair {
	var candidates []CandidatePair

	for i := 0; i < len(markets); i++ {
		for j := i + 1; j < len(markets); j++ {
			a, b := markets[i], markets[j]

			if a.Venue == b.Venue {
				continue
			}

			if a.ContractType != b.ContractType {
				continue
			}

			if len(a.Outcomes) != len(b.Outcomes) {
				continue
			}

			if isNegRiskPair(a, b) {
				continue
			}

			var flags []models.MatchFlag

			if !withinResolutionWindow(a, b, e.cfg.ResolutionWindowHours) {
				if a.ResolutionTime == nil || b.ResolutionTime == nil {
					flags = append(flags, models.FlagResolutionTimeMissing)
				} else {
					flags = append(flags, models.FlagResolutionTimeMismatch)
				}
			}

			jaccard := JaccardSimilarity(a.NormalizedTitle, b.NormalizedTitle)
			if jaccard < e.cfg.JaccardThreshold {
				continue
			}

			lev := NormalizedLevenshtein(a.NormalizedTitle, b.NormalizedTitle)
			if lev < e.cfg.LevenshteinThreshold {
				continue
			}

			candidates = append(candidates, CandidatePair{
				A:           a,
				B:           b,
				Jaccard:     jaccard,
				Levenshtein: lev,
				Flags:       flags,
			})
		}
	}

	return candidates
}

// stage2Classify uses embeddings to classify candidate pairs.
func (e *Engine) stage2Classify(ctx context.Context, candidates []CandidatePair) ([]models.EquivalenceGroup, error) {
	embeddingAvailable := e.embedder != nil && e.embedder.Available()

	if !embeddingAvailable {
		slog.Warn("OpenAI unavailable, using Stage 1 only")
		return e.classifyStage1Only(candidates), nil
	}

	texts := make([]string, 0, len(candidates)*2)
	textIndex := make(map[string]int)
	for _, c := range candidates {
		for _, m := range []models.CanonicalMarket{c.A, c.B} {
			text := embeddingText(m)
			if _, exists := textIndex[text]; !exists {
				textIndex[text] = len(texts)
				texts = append(texts, text)
			}
		}
	}

	embeddings, err := e.embedder.GetEmbeddings(ctx, texts)
	if err != nil {
		slog.Warn("embedding API failed, falling back to Stage 1 only", "error", err)
		return e.classifyStage1Only(candidates), nil
	}

	var groups []models.EquivalenceGroup

	for _, c := range candidates {
		textA := embeddingText(c.A)
		textB := embeddingText(c.B)

		embA := embeddings[textIndex[textA]]
		embB := embeddings[textIndex[textB]]

		similarity := CosineSimilarity(embA, embB)
		confidence := similarity*0.9 + c.Jaccard*0.1

		var matchMethod models.MatchMethod
		flags := append([]models.MatchFlag{}, c.Flags...)

		if similarity >= e.cfg.EmbeddingSimilarityHigh {
			matchMethod = models.MatchHybrid
		} else if similarity >= e.cfg.EmbeddingSimilarityLow {
			matchMethod = models.MatchHybrid
			flags = append(flags, models.FlagLowConfidence)
		} else {
			slog.Debug("pair rejected by embedding",
				"market_a", c.A.ID,
				"market_b", c.B.ID,
				"similarity", similarity,
			)
			continue
		}

		group := buildGroup(c.A, c.B, confidence, matchMethod, &similarity, &c.Jaccard, flags)
		groups = append(groups, group)
	}

	return groups, nil
}

func (e *Engine) classifyStage1Only(candidates []CandidatePair) []models.EquivalenceGroup {
	var groups []models.EquivalenceGroup

	for _, c := range candidates {
		confidence := c.Jaccard*0.5 + c.Levenshtein*0.5
		flags := append([]models.MatchFlag{}, c.Flags...)
		flags = append(flags, models.FlagLowConfidence, models.FlagEmbeddingUnavailable)

		group := buildGroup(c.A, c.B, confidence, models.MatchRuleBased, nil, &c.Jaccard, flags)
		groups = append(groups, group)
	}

	return groups
}

func buildGroup(a, b models.CanonicalMarket, confidence float64, method models.MatchMethod, embSim, strSim *float64, flags []models.MatchFlag) models.EquivalenceGroup {
	members := []models.CanonicalMarket{a, b}
	sort.Slice(members, func(i, j int) bool {
		return members[i].ID < members[j].ID
	})

	groupID := deterministicGroupID(members)

	var resDelta *time.Duration
	if a.ResolutionTimeUTC != nil && b.ResolutionTimeUTC != nil {
		d := a.ResolutionTimeUTC.Sub(*b.ResolutionTimeUTC)
		absDelta := time.Duration(math.Abs(float64(d)))
		resDelta = &absDelta
	}

	rationale := fmt.Sprintf(
		"Markets %s and %s matched via %s (confidence=%.3f)",
		a.ID, b.ID, method, confidence,
	)

	return models.EquivalenceGroup{
		GroupID:             groupID,
		Members:            members,
		ConfidenceScore:     confidence,
		MatchMethod:         method,
		EmbeddingSimilarity: embSim,
		StringSimilarity:    strSim,
		ResolutionDelta:     resDelta,
		MatchRationale:      rationale,
		CreatedAt:           time.Now(),
		Flags:               flags,
	}
}

func checkSettlementDivergence(group *models.EquivalenceGroup) {
	if len(group.Members) < 2 {
		return
	}
	first := group.Members[0].SettlementMechanism
	for _, m := range group.Members[1:] {
		if m.SettlementMechanism != first {
			group.Flags = append(group.Flags, models.FlagSettlementDivergence)
			return
		}
	}
}

func withinResolutionWindow(a, b models.CanonicalMarket, windowHours int) bool {
	if a.ResolutionTimeUTC == nil || b.ResolutionTimeUTC == nil {
		return true
	}
	delta := math.Abs(float64(a.ResolutionTimeUTC.Sub(*b.ResolutionTimeUTC)))
	return delta <= float64(time.Duration(windowHours)*time.Hour)
}

func isNegRiskPair(a, b models.CanonicalMarket) bool {
	aNeg := extractNegRisk(a.RawPayload)
	bNeg := extractNegRisk(b.RawPayload)

	if aNeg && bNeg && a.Venue == b.Venue {
		return true
	}
	return false
}

func extractNegRisk(payload json.RawMessage) bool {
	var data map[string]interface{}
	if err := json.Unmarshal(payload, &data); err != nil {
		return false
	}

	if gamma, ok := data["gamma"].(map[string]interface{}); ok {
		if negRisk, ok := gamma["neg_risk"].(bool); ok {
			return negRisk
		}
	}

	if negRisk, ok := data["neg_risk"].(bool); ok {
		return negRisk
	}

	return false
}

func embeddingText(m models.CanonicalMarket) string {
	desc := m.Description
	if len(desc) > 200 {
		desc = desc[:200]
	}
	return m.NormalizedTitle + " " + desc
}

func deterministicGroupID(members []models.CanonicalMarket) string {
	ids := make([]string, len(members))
	for i, m := range members {
		ids[i] = m.ID
	}
	sort.Strings(ids)
	combined := strings.Join(ids, "|")
	hash := sha256.Sum256([]byte(combined))
	return fmt.Sprintf("%x", hash[:16])
}

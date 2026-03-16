package normalizer

import (
	"crypto/sha256"
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"equinox/adapters"
	"equinox/models"
)

// Normalizer transforms raw venue data into CanonicalMarket objects.
type Normalizer struct{}

func New() *Normalizer {
	return &Normalizer{}
}

// Normalize converts raw markets from any venue into canonical form.
func (n *Normalizer) Normalize(raws []adapters.RawMarket) ([]models.CanonicalMarket, []error) {
	var results []models.CanonicalMarket
	var errs []error

	for _, raw := range raws {
		var cm *models.CanonicalMarket
		var err error

		switch raw.Venue {
		case models.VenueKalshi:
			cm, err = normalizeKalshi(raw)
		case models.VenuePolymarket:
			cm, err = normalizePolymarket(raw)
		default:
			errs = append(errs, fmt.Errorf("unknown venue %s for market %s", raw.Venue, raw.NativeID))
			continue
		}

		if err != nil {
			errs = append(errs, fmt.Errorf("normalize %s:%s: %w", raw.Venue, raw.NativeID, err))
			continue
		}

		results = append(results, *cm)
	}

	return results, errs
}

var (
	punctuationRe = regexp.MustCompile(`[^\w\s]`)
	whitespaceRe  = regexp.MustCompile(`\s+`)
)

// NormalizeTitle performs the title normalization pipeline:
// lowercase → strip punctuation → collapse whitespace → trim.
func NormalizeTitle(title string) string {
	s := strings.ToLower(title)
	s = punctuationRe.ReplaceAllString(s, "")
	s = whitespaceRe.ReplaceAllString(s, " ")
	s = strings.TrimSpace(s)

	var stemmed []string
	for _, word := range strings.Fields(s) {
		stemmed = append(stemmed, simpleStem(word))
	}
	return strings.Join(stemmed, " ")
}

// simpleStem applies minimal suffix-stripping for English.
// Not a full Porter stemmer — covers common cases to improve token overlap.
func simpleStem(word string) string {
	suffixes := []string{"ing", "tion", "ness", "ment", "able", "ible", "ful", "less", "ous", "ive", "ize", "ise"}
	for _, suffix := range suffixes {
		if len(word) > len(suffix)+3 && strings.HasSuffix(word, suffix) {
			return strings.TrimSuffix(word, suffix)
		}
	}
	if strings.HasSuffix(word, "ed") && len(word) > 6 {
		stem := strings.TrimSuffix(word, "ed")
		if !strings.HasSuffix(stem, "e") {
			return stem
		}
	}
	if strings.HasSuffix(word, "s") && !strings.HasSuffix(word, "ss") && len(word) > 4 {
		return strings.TrimSuffix(word, "s")
	}
	return word
}

// ComputeRulesHash generates SHA-256 of normalized description text.
func ComputeRulesHash(description string) string {
	normalized := strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return ' '
		}
		return unicode.ToLower(r)
	}, description)
	normalized = whitespaceRe.ReplaceAllString(strings.TrimSpace(normalized), " ")
	hash := sha256.Sum256([]byte(normalized))
	return fmt.Sprintf("%x", hash)
}

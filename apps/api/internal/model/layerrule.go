package model

import (
	"slices"
	"strings"

	"archmap/api/internal/platform/apperr"
)

// LayerRule limits an element kind (its Class) to some layers (names match ignoring case).
type LayerRule struct {
	Class  string
	Layers []string
}

// MaxRuleLayers bounds the layers of one rule (same as the layers of a view).
const MaxRuleLayers = 12

// CheckLayer returns an error when the element's layer is not allowed by the
// rules that apply to it. An element without a layer is always fine.
func CheckLayer(rules []LayerRule, e Element) error {
	if e.Layer == "" {
		return nil
	}
	for _, r := range rules {
		if len(r.Layers) == 0 || string(e.Kind) != r.Class {
			continue
		}
		if !slices.ContainsFunc(r.Layers, func(l string) bool { return strings.EqualFold(l, e.Layer) }) {
			return apperr.Invalid("%s elements can only be placed on the layers %s, not %q", r.Class, strings.Join(r.Layers, ", "), e.Layer)
		}
	}
	return nil
}

// NormalizeLayerRules validates rules and returns them cleaned: known classes,
// each at most once, layer names trimmed and de-duplicated; rules without layers
// (unrestricted) are dropped.
func NormalizeLayerRules(in []LayerRule) ([]LayerRule, error) {
	out := make([]LayerRule, 0, len(in))
	seen := map[string]bool{}
	for _, r := range in {
		if !ElementKind(r.Class).Valid() {
			return nil, apperr.Invalid("unknown element class %q", r.Class)
		}
		if seen[r.Class] {
			return nil, apperr.Invalid("class %q appears twice", r.Class)
		}
		seen[r.Class] = true
		layers := make([]string, 0, len(r.Layers))
		for _, l := range r.Layers {
			l = strings.TrimSpace(l)
			if l == "" || slices.ContainsFunc(layers, func(x string) bool { return strings.EqualFold(x, l) }) {
				continue
			}
			if err := ValidateLayer(l); err != nil {
				return nil, err
			}
			layers = append(layers, l)
		}
		if len(layers) > MaxRuleLayers {
			return nil, apperr.Invalid("a rule can name at most %d layers", MaxRuleLayers)
		}
		if len(layers) > 0 {
			out = append(out, LayerRule{Class: r.Class, Layers: layers})
		}
	}
	return out, nil
}

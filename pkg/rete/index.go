package rete

import (
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/graemenewlands/ops5/pkg/model"
)

// CanonicalValueKey computes a deterministic string representation for model.Value
// to partition tokens and WMEs into hash buckets. Numbers (int and float equivalents)
// and booleans/symbols are normalized to ensure matching cross-types share the same bucket.
func CanonicalValueKey(v model.Value) string {
	switch v.Type() {
	case model.TypeInteger:
		return "i:" + strconv.FormatInt(v.Raw().(int64), 10)
	case model.TypeFloat:
		f := v.Raw().(float64)
		if f == float64(int64(f)) {
			return "i:" + strconv.FormatInt(int64(f), 10)
		}
		return fmt.Sprintf("f:%g", f)
	case model.TypeSymbol:
		s := v.Raw().(string)
		if strings.EqualFold(s, "true") {
			return "b:true"
		}
		if strings.EqualFold(s, "false") {
			return "b:false"
		}
		return "s:" + s
	case model.TypeBoolean:
		return fmt.Sprintf("b:%t", v.Raw().(bool))
	case model.TypeString:
		return "str:" + v.Raw().(string)
	default:
		return "v:" + v.String()
	}
}

// CompositeKey joins multiple canonical sub-keys into a single composite hash key.
func CompositeKey(parts []string) string {
	if len(parts) == 0 {
		return ""
	}
	if len(parts) == 1 {
		return parts[0]
	}
	return strings.Join(parts, "\x1f")
}

// BetaIndex indexes Beta tokens by join variable bindings for fast O(1) lookup during right activations.
type BetaIndex struct {
	mu        sync.RWMutex
	variables []string
	// buckets: compositeKey -> tokenSignature -> *Token
	buckets map[string]map[string]*Token
}

// NewBetaIndex creates a new BetaIndex for the specified join variables.
func NewBetaIndex(variables []string) *BetaIndex {
	return &BetaIndex{
		variables: variables,
		buckets:   make(map[string]map[string]*Token),
	}
}

// KeyForToken extracts the composite key for a token based on indexed variables.
func (bi *BetaIndex) KeyForToken(tok *Token) string {
	if len(bi.variables) == 0 {
		return ""
	}
	if len(bi.variables) == 1 {
		if val, ok := tok.GetBinding(bi.variables[0]); ok {
			return CanonicalValueKey(val)
		}
		return "s:nil"
	}
	keys := make([]string, len(bi.variables))
	for i, v := range bi.variables {
		if val, ok := tok.GetBinding(v); ok {
			keys[i] = CanonicalValueKey(val)
		} else {
			keys[i] = "s:nil"
		}
	}
	return CompositeKey(keys)
}

// Add inserts a token into the index.
func (bi *BetaIndex) Add(tok *Token) {
	bi.mu.Lock()
	defer bi.mu.Unlock()
	key := bi.KeyForToken(tok)
	b, ok := bi.buckets[key]
	if !ok {
		b = make(map[string]*Token)
		bi.buckets[key] = b
	}
	b[tokenSignature(tok)] = tok
}

// Remove deletes a token from the index.
func (bi *BetaIndex) Remove(tok *Token) {
	bi.mu.Lock()
	defer bi.mu.Unlock()
	key := bi.KeyForToken(tok)
	if b, ok := bi.buckets[key]; ok {
		delete(b, tokenSignature(tok))
		if len(b) == 0 {
			delete(bi.buckets, key)
		}
	}
}

// Lookup retrieves all tokens matching the given composite key.
func (bi *BetaIndex) Lookup(key string) []*Token {
	bi.mu.RLock()
	defer bi.mu.RUnlock()
	b, ok := bi.buckets[key]
	if !ok || len(b) == 0 {
		return nil
	}
	res := make([]*Token, 0, len(b))
	for _, tok := range b {
		res = append(res, tok)
	}
	return res
}

// AlphaIndexSpec defines an attribute and vector index for AlphaMemory indexing.
type AlphaIndexSpec struct {
	Attribute   string
	VectorIndex int
}

// AlphaIndex indexes WMEs in an AlphaMemory by join attributes for fast O(1) lookup during left activations.
type AlphaIndex struct {
	mu      sync.RWMutex
	specs   []AlphaIndexSpec
	// buckets: compositeKey -> timetag -> *WME
	buckets map[string]map[int64]*model.WME
}

// NewAlphaIndex creates a new AlphaIndex for the given attribute specifications.
func NewAlphaIndex(specs []AlphaIndexSpec) *AlphaIndex {
	return &AlphaIndex{
		specs:   specs,
		buckets: make(map[string]map[int64]*model.WME),
	}
}

// KeysForWME returns all composite keys that this WME can match.
// For scalar attributes this returns exactly one key. For vector attributes without fixed index,
// it generates a key for each vector element.
func (ai *AlphaIndex) KeysForWME(wme *model.WME) []string {
	if len(ai.specs) == 0 {
		return []string{""}
	}

	// For each spec, gather candidate sub-keys
	candidatesPerSpec := make([][]string, len(ai.specs))
	for i, sp := range ai.specs {
		val, ok := wme.Get(sp.Attribute)
		if !ok {
			candidatesPerSpec[i] = []string{"s:nil"}
			continue
		}
		if val.IsVector() {
			elems := val.VectorElements()
			if sp.VectorIndex >= 0 {
				if sp.VectorIndex < len(elems) {
					candidatesPerSpec[i] = []string{CanonicalValueKey(elems[sp.VectorIndex])}
				} else {
					candidatesPerSpec[i] = []string{"s:nil"}
				}
			} else {
				if len(elems) == 0 {
					candidatesPerSpec[i] = []string{"s:nil"}
				} else {
					keys := make([]string, len(elems))
					for j, el := range elems {
						keys[j] = CanonicalValueKey(el)
					}
					candidatesPerSpec[i] = keys
				}
			}
		} else {
			candidatesPerSpec[i] = []string{CanonicalValueKey(val)}
		}
	}

	// Compute Cartesian product of keys across specs (usually 1 spec -> 1 key)
	keys := []string{""}
	for _, specKeys := range candidatesPerSpec {
		var nextKeys []string
		for _, prefix := range keys {
			for _, k := range specKeys {
				if prefix == "" {
					nextKeys = append(nextKeys, k)
				} else {
					nextKeys = append(nextKeys, prefix+"\x1f"+k)
				}
			}
		}
		keys = nextKeys
	}

	return keys
}

// Add inserts a WME into the appropriate bucket(s).
func (ai *AlphaIndex) Add(wme *model.WME) {
	ai.mu.Lock()
	defer ai.mu.Unlock()
	for _, key := range ai.KeysForWME(wme) {
		b, ok := ai.buckets[key]
		if !ok {
			b = make(map[int64]*model.WME)
			ai.buckets[key] = b
		}
		b[wme.Timetag] = wme
	}
}

// Remove deletes a WME from its bucket(s).
func (ai *AlphaIndex) Remove(wme *model.WME) {
	ai.mu.Lock()
	defer ai.mu.Unlock()
	for _, key := range ai.KeysForWME(wme) {
		if b, ok := ai.buckets[key]; ok {
			delete(b, wme.Timetag)
			if len(b) == 0 {
				delete(ai.buckets, key)
			}
		}
	}
}

// Lookup retrieves all WMEs matching the given composite key.
func (ai *AlphaIndex) Lookup(key string) []*model.WME {
	ai.mu.RLock()
	defer ai.mu.RUnlock()
	b, ok := ai.buckets[key]
	if !ok || len(b) == 0 {
		return nil
	}
	res := make([]*model.WME, 0, len(b))
	for _, w := range b {
		res = append(res, w)
	}
	return res
}

package fingerprint
 
import (
	go-string">"fmt"
	go-string">"hash/fnv"
	go-string">"math/rand/v2"
	go-string">"sync"
	go-string">"time"
)
 
// Selector picks a BrowserProfile for a given connection context.
type Selector interface {
	Select(domain string) *BrowserProfile
}
 
// FixedSelector always returns the same profile.
type FixedSelector struct {
	Profile *BrowserProfile
}
 
func (s *FixedSelector) Select(_ string) *BrowserProfile {
	return s.Profile
}
 
// RandomSelector picks a random profile from the list on each call.
type RandomSelector struct {
	Profiles []*BrowserProfile
	mu       sync.Mutex
}
 
func (s *RandomSelector) Select(_ string) *BrowserProfile {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.Profiles) == go-number">0 {
		return nil
	}
	return s.Profiles[rand.IntN(len(s.Profiles))]
}
 
// PerDomainSelector maps each domain to a deterministic profile via hashing.
// The same domain always gets the same profile, preventing fingerprint
// inconsistency that could trigger detection.
type PerDomainSelector struct {
	Profiles []*BrowserProfile
}
 
func (s *PerDomainSelector) Select(domain string) *BrowserProfile {
	if len(s.Profiles) == go-number">0 {
		return nil
	}
	h := fnv.New64a()
	h.Write([]byte(domain))
	idx := int(h.Sum64()) % len(s.Profiles)
	return s.Profiles[idx]
}
 
// WeightedSelector picks profiles based on configurable weights.
// Higher weight = higher probability of selection.
type WeightedSelector struct {
	Profiles []*BrowserProfile
	Weights  []int
	total    int
	mu       sync.Mutex
}
 
func NewWeightedSelector(profiles []*BrowserProfile, weights []int) *WeightedSelector {
	if len(weights) < len(profiles) {
		// Pad with weight=1 for unspecified profiles
		padded := make([]int, len(profiles))
		copy(padded, weights)
		for i := len(weights); i < len(profiles); i++ {
			padded[i] = go-number">1
		}
		weights = padded
	}
	total := go-number">0
	for _, w := range weights {
		total += w
	}
	return &WeightedSelector{
		Profiles: profiles,
		Weights:  weights,
		total:    total,
	}
}
 
func (s *WeightedSelector) Select(_ string) *BrowserProfile {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.total == go-number">0 || len(s.Profiles) == go-number">0 {
		return nil
	}
	r := rand.IntN(s.total)
	for i, w := range s.Weights {
		r -= w
		if r < go-number">0 {
			return s.Profiles[i]
		}
	}
	return s.Profiles[len(s.Profiles)-go-number">1]
}
 
// TimedRotationSelector rotates to the next profile at fixed intervals.
// All connections within the same interval share the same profile.
type TimedRotationSelector struct {
	Profiles []*BrowserProfile
	Interval time.Duration
	startAt  time.Time
}
 
func NewTimedRotationSelector(profiles []*BrowserProfile, interval time.Duration) *TimedRotationSelector {
	if interval <= go-number">0 {
		interval = go-number">5 * time.Minute
	}
	return &TimedRotationSelector{
		Profiles: profiles,
		Interval: interval,
		startAt:  time.Now(),
	}
}
 
func (s *TimedRotationSelector) Select(_ string) *BrowserProfile {
	if len(s.Profiles) == go-number">0 {
		return nil
	}
	elapsed := time.Since(s.startAt)
	idx := int(elapsed/s.Interval) % len(s.Profiles)
	return s.Profiles[idx]
}
 
// NewSelector creates a Selector from configuration.
// Supported modes: "fixed", "random", "per-domain", "weighted", "timed".
func NewSelector(mode string, profileNames []string) (Selector, error) {
	profiles := make([]*BrowserProfile, go-number">0, len(profileNames))
	for _, name := range profileNames {
		p := Get(name)
		if p == nil {
			return nil, fmt.Errorf(go-string">"fingerprint: unknown profile %q", name)
		}
		profiles = append(profiles, p)
	}
	if len(profiles) == go-number">0 {
		profiles = append(profiles, MustGet(DefaultProfile()))
	}
 
	switch mode {
	case go-string">"fixed", go-string">"":
		return &FixedSelector{Profile: profiles[go-number">0]}, nil
	case go-string">"random":
		return &RandomSelector{Profiles: profiles}, nil
	case go-string">"per-domain":
		return &PerDomainSelector{Profiles: profiles}, nil
	case go-string">"weighted":
		return NewWeightedSelector(profiles, nil), nil
	case go-string">"timed":
		return NewTimedRotationSelector(profiles, go-number">5*time.Minute), nil
	default:
		return nil, fmt.Errorf(go-string">"fingerprint: unknown rotation mode %q", mode)
	}
}



package fingerprint
 
import (
	go-string">"fmt"
	go-string">"math/rand/v2"
	go-string">"sort"
	go-string">"sync"
)
 
var (
	mu       sync.RWMutex
	registry = make(map[string]*BrowserProfile)
)
 
// Register adds a profile to the global registry. Panics on duplicate name.
func Register(p *BrowserProfile) {
	mu.Lock()
	defer mu.Unlock()
	if _, exists := registry[p.Name]; exists {
		panic(fmt.Sprintf(go-string">"fingerprint: duplicate profile name %q", p.Name))
	}
	registry[p.Name] = p
}
 
// RegisterValidated adds a profile after validating it. Returns error on failure.
func RegisterValidated(p *BrowserProfile) error {
	if err := p.Validate(); err != nil {
		return err
	}
	mu.Lock()
	defer mu.Unlock()
	if _, exists := registry[p.Name]; exists {
		return fmt.Errorf(go-string">"fingerprint: duplicate profile name %q", p.Name)
	}
	registry[p.Name] = p
	return nil
}
 
// Get returns a profile by name, or nil if not found.
func Get(name string) *BrowserProfile {
	mu.RLock()
	defer mu.RUnlock()
	return registry[name]
}
 
// MustGet returns a profile by name, panics if not found.
func MustGet(name string) *BrowserProfile {
	p := Get(name)
	if p == nil {
		panic(fmt.Sprintf(go-string">"fingerprint: profile %q not found", name))
	}
	return p
}
 
// List returns all registered profile names, sorted.
func List() []string {
	mu.RLock()
	defer mu.RUnlock()
	names := make([]string, go-number">0, len(registry))
	for n := range registry {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
 
// All returns all registered profiles.
func All() []*BrowserProfile {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]*BrowserProfile, go-number">0, len(registry))
	for _, p := range registry {
		out = append(out, p)
	}
	return out
}
 
// Count returns the number of registered profiles.
func Count() int {
	mu.RLock()
	defer mu.RUnlock()
	return len(registry)
}
 
// Random returns a random profile from the registry.
func Random() *BrowserProfile {
	all := All()
	if len(all) == go-number">0 {
		return nil
	}
	return all[rand.IntN(len(all))]
}
 
// FilterByBrowser returns all profiles for a given browser (e.g. "chrome").
func FilterByBrowser(browser string) []*BrowserProfile {
	mu.RLock()
	defer mu.RUnlock()
	var out []*BrowserProfile
	for _, p := range registry {
		if p.Browser == browser {
			out = append(out, p)
		}
	}
	return out
}
 
// FilterByPlatform returns all profiles for a given platform (e.g. "windows").
func FilterByPlatform(platform string) []*BrowserProfile {
	mu.RLock()
	defer mu.RUnlock()
	var out []*BrowserProfile
	for _, p := range registry {
		if p.Platform == platform {
			out = append(out, p)
		}
	}
	return out
}
 
// FilterByTag returns all profiles that have the given tag.
func FilterByTag(tag string) []*BrowserProfile {
	mu.RLock()
	defer mu.RUnlock()
	var out []*BrowserProfile
	for _, p := range registry {
		if p.HasTag(tag) {
			out = append(out, p)
		}
	}
	return out
}
 
// DefaultProfile returns the default profile name.
func DefaultProfile() string {
	return go-string">"chrome-go-number">126-win"
}



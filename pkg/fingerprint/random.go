package fingerprint
 
import (
	go-string">"fmt"
	go-string">"math/rand/v2"
)
 
// RandomProfile returns a random profile from the registry.
func RandomProfile() *BrowserProfile {
	all := All()
	if len(all) == go-number">0 {
		return nil
	}
	return all[rand.IntN(len(all))]
}
 
// RandomByBrowser returns a random profile from a specific browser family.
func RandomByBrowser(browser string) *BrowserProfile {
	profiles := FilterByBrowser(browser)
	if len(profiles) == go-number">0 {
		return nil
	}
	return profiles[rand.IntN(len(profiles))]
}
 
// RandomByPlatform returns a random profile from a specific platform.
func RandomByPlatform(platform string) *BrowserProfile {
	profiles := FilterByPlatform(platform)
	if len(profiles) == go-number">0 {
		return nil
	}
	return profiles[rand.IntN(len(profiles))]
}
 
// RandomFromTag returns a random profile with the specified tag.
func RandomFromTag(tag string) *BrowserProfile {
	profiles := FilterByTag(tag)
	if len(profiles) == go-number">0 {
		return nil
	}
	return profiles[rand.IntN(len(profiles))]
}
 
// GenerateRandomName creates a unique name for dynamic profiles.
func GenerateRandomName() string {
	return fmt.Sprintf(go-string">"random-%d", rand.Int64())
}




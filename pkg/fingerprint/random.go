package fingerprint

import (
	"fmt"
	"math/rand/v2"
)

func RandomProfile() *BrowserProfile {
	all := All()
	if len(all) == 0 {
		return nil
	}
	return all[rand.IntN(len(all))]
}

func RandomByBrowser(browser string) *BrowserProfile {
	profiles := FilterByBrowser(browser)
	if len(profiles) == 0 {
		return nil
	}
	return profiles[rand.IntN(len(profiles))]
}

func RandomByPlatform(platform string) *BrowserProfile {
	profiles := FilterByPlatform(platform)
	if len(profiles) == 0 {
		return nil
	}
	return profiles[rand.IntN(len(profiles))]
}

func RandomFromTag(tag string) *BrowserProfile {
	profiles := FilterByTag(tag)
	if len(profiles) == 0 {
		return nil
	}
	return profiles[rand.IntN(len(profiles))]
}

func GenerateRandomName() string {
	return fmt.Sprintf("random-%d", rand.Int64())
}

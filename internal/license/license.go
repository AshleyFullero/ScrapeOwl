package license

// Feature represents an optional/premium feature
type Feature string

const (
	FeatureCloudSync     Feature = "cloud_sync"
	FeatureTeamAccess    Feature = "team_access"
	FeatureAdvancedProxy Feature = "advanced_proxy"
	FeatureAPIAccess     Feature = "api_access"
	FeatureWebhooks      Feature = "webhooks"
	FeatureSSO           Feature = "sso"
)

// Tier represents the license tier
type Tier string

const (
	TierOpenSource Tier = "opensource"
	TierPro        Tier = "pro"
	TierEnterprise Tier = "enterprise"
)

// License holds the current license information
type License struct {
	Tier       Tier   `json:"tier"`
	Licensee   string `json:"licensee"`
	ValidUntil string `json:"valid_until"`
}

// current is the active license (always open-source in this build)
var current = &License{
	Tier:       TierOpenSource,
	Licensee:   "Open Source User",
	ValidUntil: "unlimited",
}

// Get returns the current license
func Get() *License {
	return current
}

// IsFeatureEnabled returns whether a given feature is available.
// In the open-source build, all core features are enabled.
// Cloud/enterprise features are disabled pending a license.
func IsFeatureEnabled(feature Feature) bool {
	switch feature {
	// Core features always enabled in open-source
	case FeatureAPIAccess, FeatureWebhooks, FeatureAdvancedProxy:
		return true

	// Cloud/enterprise features require a paid license (stub - future)
	case FeatureCloudSync, FeatureTeamAccess, FeatureSSO:
		return current.Tier == TierPro || current.Tier == TierEnterprise

	default:
		return true
	}
}

// IsOpenSource returns true if running the open-source build
func IsOpenSource() bool {
	return current.Tier == TierOpenSource
}

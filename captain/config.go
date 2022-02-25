package captain

import (
	"sync"

	"github.com/safing/portbase/utils"

	"github.com/safing/portbase/config"
	"github.com/safing/portmaster/profile"
	"github.com/safing/portmaster/profile/endpoints"
)

var (
	// CfgOptionEnableSPNKey is the configuration key for the SPN module.
	CfgOptionEnableSPNKey   = "spn/enable"
	cfgOptionEnableSPNOrder = 128

	CfgOptionHomeHubPolicyKey   = "spn/homePolicy"
	cfgOptionHomeHubPolicy      config.StringArrayOption
	cfgOptionHomeHubPolicyOrder = 145

	CfgOptionDNSExitHubPolicyKey   = "spn/dnsExitPolicy"
	cfgOptionDNSExitHubPolicy      config.StringArrayOption
	cfgOptionDNSExitHubPolicyOrder = 147

	// Special Access Code.
	cfgOptionSpecialAccessCodeKey     = "spn/specialAccessCode"
	cfgOptionSpecialAccessCodeDefault = "none"
	cfgOptionSpecialAccessCode        config.StringOption //nolint:unused // Linter, you drunk?
	cfgOptionSpecialAccessCodeOrder   = 160
)

func prepConfig() error {
	// Home Node Rules
	err := config.Register(&config.Option{
		Name: "Home Node Rules",
		Key:  CfgOptionHomeHubPolicyKey,
		Description: `Customize which countries should or should not be used for your Home Node. The Home Node is your entry into the SPN. You connect directly to it and all your connections are routed through it.

By default, the Portmaster tries to choose the nearest node as your Home Node in order to reduce your exposure to the open Internet.

Reconnect to the SPN in order to apply new rules.`,
		Help:           profile.SPNRulesHelp,
		OptType:        config.OptTypeStringArray,
		ExpertiseLevel: config.ExpertiseLevelExpert,
		DefaultValue:   []string{},
		Annotations: config.Annotations{
			config.StackableAnnotation:                   true,
			config.CategoryAnnotation:                    "Routing",
			config.DisplayOrderAnnotation:                cfgOptionHomeHubPolicyOrder,
			config.DisplayHintAnnotation:                 endpoints.DisplayHintEndpointList,
			config.QuickSettingsAnnotation:               profile.SPNRulesQuickSettings,
			endpoints.EndpointListVerdictNamesAnnotation: profile.SPNRulesVerdictNames,
		},
		ValidationRegex: endpoints.ListEntryValidationRegex,
		ValidationFunc:  endpoints.ValidateEndpointListConfigOption,
	})
	if err != nil {
		return err
	}
	cfgOptionHomeHubPolicy = config.Concurrent.GetAsStringArray(CfgOptionHomeHubPolicyKey, []string{})

	// DNS Exit Node Rules
	err = config.Register(&config.Option{
		Name: "DNS Exit Node Rules",
		Key:  CfgOptionDNSExitHubPolicyKey,
		Description: `Customize which countries should or should not be used as DNS Exit Nodes.

By default, the Portmaster will exit DNS requests directly at your Home Node in order to keep them fast and close to your location. This is important, as DNS resolution often takes your approximate location into account when deciding which optimized DNS records are returned to you. As the Portmaster encrypts your DNS requests by default, you effectively gain a two-hop security level for your DNS requests in order to protect your privacy.

This setting mainly exists for when you need to simulate your presence in another location on a lower level too. This might be necessary to defeat more intelligent geo-blocking systems.`,
		Help:            profile.SPNRulesHelp,
		OptType:         config.OptTypeStringArray,
		RequiresRestart: true,
		ExpertiseLevel:  config.ExpertiseLevelExpert,
		DefaultValue:    []string{},
		Annotations: config.Annotations{
			config.StackableAnnotation:                   true,
			config.CategoryAnnotation:                    "Routing",
			config.DisplayOrderAnnotation:                cfgOptionDNSExitHubPolicyOrder,
			config.DisplayHintAnnotation:                 endpoints.DisplayHintEndpointList,
			config.QuickSettingsAnnotation:               profile.SPNRulesQuickSettings,
			endpoints.EndpointListVerdictNamesAnnotation: profile.SPNRulesVerdictNames,
		},
		ValidationRegex: endpoints.ListEntryValidationRegex,
		ValidationFunc:  endpoints.ValidateEndpointListConfigOption,
	})
	if err != nil {
		return err
	}
	cfgOptionDNSExitHubPolicy = config.Concurrent.GetAsStringArray(CfgOptionDNSExitHubPolicyKey, []string{})

	err = config.Register(&config.Option{
		Name:         "Special Access Code",
		Key:          cfgOptionSpecialAccessCodeKey,
		Description:  "Special Access Codes grant access to the SPN for testing or evaluation purposes.",
		OptType:      config.OptTypeString,
		DefaultValue: cfgOptionSpecialAccessCodeDefault,
		Annotations: config.Annotations{
			config.DisplayOrderAnnotation: cfgOptionSpecialAccessCodeOrder,
			config.CategoryAnnotation:     "Advanced",
		},
	})
	if err != nil {
		return err
	}
	cfgOptionSpecialAccessCode = config.Concurrent.GetAsString(cfgOptionSpecialAccessCodeKey, "")

	return nil
}

var (
	homeHubPolicy     endpoints.Endpoints
	homeHubPolicyData []string
	homeHubPolcyLock  sync.Mutex
)

func getHomeHubPolicy() (endpoints.Endpoints, error) {
	homeHubPolcyLock.Lock()
	defer homeHubPolcyLock.Unlock()

	// Get config data and check if it changed.
	newData := cfgOptionHomeHubPolicy()
	if utils.StringSliceEqual(newData, homeHubPolicyData) {
		return homeHubPolicy, nil
	}

	// Check if policy is empty and return without parsing.
	if len(newData) == 0 {
		homeHubPolicy = nil
		return homeHubPolicy, nil
	}

	// Parse policy.
	policy, err := endpoints.ParseEndpoints(newData)
	if err != nil {
		homeHubPolicy = nil
		return nil, err
	}

	// Save and return the new policy.
	homeHubPolicy = policy
	return homeHubPolicy, nil
}

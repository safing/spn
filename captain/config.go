package captain

import "github.com/safing/portbase/config"

var (
	// CfgOptionEnableSPNKey is the configuration key for the SPN module.
	CfgOptionEnableSPNKey   = "spn/enable"
	cfgOptionEnableSPNOrder = 128

	// Special Access Code.
	cfgOptionSpecialAccessCodeKey     = "spn/specialAccessCode"
	cfgOptionSpecialAccessCodeDefault = "none"
	cfgOptionSpecialAccessCode        config.StringOption //nolint:unused // Linter, you drunk?
	cfgOptionSpecialAccessCodeOrder   = 144
)

func prepConfig() error {
	err := config.Register(&config.Option{
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

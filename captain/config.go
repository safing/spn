package captain

import "github.com/safing/portbase/config"

var (
	CfgOptionEnableSPNKey   = "spn/enable"
	cfgOptionEnableSPNOrder = 500

	// Special Access Code
	cfgOptionSpecialAccessCodeKey     = "spn/specialAccessCode"
	cfgOptionSpecialAccessCodeDefault = "none"
	cfgOptionSpecialAccessCode        config.StringOption
	cfgOptionSpecialAccessCodeOrder   = 501
)

func prepConfig() error {
	err := config.Register(&config.Option{
		Name:         "Special Access Code",
		Key:          cfgOptionSpecialAccessCodeKey,
		Description:  "Special Access Codes grant access to the SPN for testing or evaluation purposes.",
		Order:        cfgOptionSpecialAccessCodeOrder,
		OptType:      config.OptTypeString,
		DefaultValue: cfgOptionSpecialAccessCodeDefault,
	})
	if err != nil {
		return err
	}

	cfgOptionSpecialAccessCode = config.Concurrent.GetAsString(cfgOptionSpecialAccessCodeKey, "")

	return nil
}

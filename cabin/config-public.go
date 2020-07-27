package cabin

import (
	"net"
	"os"

	"github.com/safing/portbase/config"
	"github.com/safing/portbase/log"
	"github.com/safing/portmaster/netenv"
	"github.com/safing/spn/hub"
)

// Configuration Keys
var (
	// Name of the node
	publicCfgOptionNameKey     = "spn/publicHub/name"
	publicCfgOptionName        config.StringOption
	publicCfgOptionNameDefault = ""
	publicCfgOptionNameOrder   = 512

	// Person or organisation, who is in control of the node (should be same for all nodes of this person or organisation)
	publicCfgOptionGroupKey     = "spn/publicHub/group"
	publicCfgOptionGroup        config.StringOption
	publicCfgOptionGroupDefault = ""
	publicCfgOptionGroupOrder   = 513

	// Contact possibility  (recommended, but optional)
	publicCfgOptionContactAddressKey     = "spn/publicHub/contactAddress"
	publicCfgOptionContactAddress        config.StringOption
	publicCfgOptionContactAddressDefault = ""
	publicCfgOptionContactAddressOrder   = 514

	// Type of service of the contact address, if not email
	publicCfgOptionContactServiceKey     = "spn/publicHub/contactService"
	publicCfgOptionContactService        config.StringOption
	publicCfgOptionContactServiceDefault = ""
	publicCfgOptionContactServiceOrder   = 515

	// Hosters - supply chain (reseller, hosting provider, datacenter operator, ...)
	publicCfgOptionHostersKey     = "spn/publicHub/hosters"
	publicCfgOptionHosters        config.StringArrayOption
	publicCfgOptionHostersDefault = []string{}
	publicCfgOptionHostersOrder   = 516

	// Datacenter
	// Format: CC-COMPANY-INTERNALCODE
	// Eg: DE-Hetzner-FSN1-DC5
	publicCfgOptionDatacenterKey     = "spn/publicHub/datacenter"
	publicCfgOptionDatacenter        config.StringOption
	publicCfgOptionDatacenterDefault = ""
	publicCfgOptionDatacenterOrder   = 517

	// Network Location and Access

	// IPv4 must be global and accessible
	publicCfgOptionIPv4Key     = "spn/publicHub/ip4"
	publicCfgOptionIPv4        config.StringOption
	publicCfgOptionIPv4Default = ""
	publicCfgOptionIPv4Order   = 518

	// IPv6 must be global and accessible
	publicCfgOptionIPv6Key     = "spn/publicHub/ip6"
	publicCfgOptionIPv6        config.StringOption
	publicCfgOptionIPv6Default = ""
	publicCfgOptionIPv6Order   = 519

	// Transports
	publicCfgOptionTransportsKey     = "spn/publicHub/transports"
	publicCfgOptionTransports        config.StringArrayOption
	publicCfgOptionTransportsDefault = []string{"tcp:17", "kcp:17"}
	publicCfgOptionTransportsOrder   = 520

	// Entry Policy
	publicCfgOptionEntryKey     = "spn/publicHub/entry"
	publicCfgOptionEntry        config.StringArrayOption
	publicCfgOptionEntryDefault = []string{}
	publicCfgOptionEntryOrder   = 521

	// Exit Policy
	publicCfgOptionExitKey     = "spn/publicHub/exit"
	publicCfgOptionExit        config.StringArrayOption
	publicCfgOptionExitDefault = []string{"- * TCP/25"}
	publicCfgOptionExitOrder   = 522
)

func prepConfig() error {
	err := config.Register(&config.Option{
		Name:           "Name",
		Key:            publicCfgOptionNameKey,
		Description:    "Human readable name of the Hub.",
		Order:          publicCfgOptionNameOrder,
		OptType:        config.OptTypeString,
		ExpertiseLevel: config.ExpertiseLevelExpert,
		DefaultValue:   publicCfgOptionNameDefault,
	})
	if err != nil {
		return err
	}
	publicCfgOptionName = config.GetAsString(publicCfgOptionNameKey, publicCfgOptionNameDefault)

	err = config.Register(&config.Option{
		Name:           "Group",
		Key:            publicCfgOptionGroupKey,
		Description:    "Name of the hub group this Hub belongs to.",
		Order:          publicCfgOptionGroupOrder,
		OptType:        config.OptTypeString,
		ExpertiseLevel: config.ExpertiseLevelExpert,
		DefaultValue:   publicCfgOptionGroupDefault,
	})
	if err != nil {
		return err
	}
	publicCfgOptionGroup = config.GetAsString(publicCfgOptionGroupKey, publicCfgOptionGroupDefault)

	err = config.Register(&config.Option{
		Name:           "Contact Address",
		Key:            publicCfgOptionContactAddressKey,
		Description:    "Contact address where the Hub operator can be reached.",
		Order:          publicCfgOptionContactAddressOrder,
		OptType:        config.OptTypeString,
		ExpertiseLevel: config.ExpertiseLevelExpert,
		DefaultValue:   publicCfgOptionContactAddressDefault,
	})
	if err != nil {
		return err
	}
	publicCfgOptionContactAddress = config.GetAsString(publicCfgOptionContactAddressKey, publicCfgOptionContactAddressDefault)

	err = config.Register(&config.Option{
		Name:           "Contact Service",
		Key:            publicCfgOptionContactServiceKey,
		Description:    "Name of the service the contact address corresponds to, if not email.",
		Order:          publicCfgOptionContactServiceOrder,
		OptType:        config.OptTypeString,
		ExpertiseLevel: config.ExpertiseLevelExpert,
		DefaultValue:   publicCfgOptionContactServiceDefault,
	})
	if err != nil {
		return err
	}
	publicCfgOptionContactService = config.GetAsString(publicCfgOptionContactServiceKey, publicCfgOptionContactServiceDefault)

	err = config.Register(&config.Option{
		Name:           "Hosters",
		Key:            publicCfgOptionHostersKey,
		Description:    "List of all involved entities and organisations that are involved in hosting this Hub.",
		Order:          publicCfgOptionHostersOrder,
		OptType:        config.OptTypeStringArray,
		ExpertiseLevel: config.ExpertiseLevelExpert,
		DefaultValue:   publicCfgOptionHostersDefault,
	})
	if err != nil {
		return err
	}
	publicCfgOptionHosters = config.GetAsStringArray(publicCfgOptionHostersKey, publicCfgOptionHostersDefault)

	err = config.Register(&config.Option{
		Name:           "Datacenter",
		Key:            publicCfgOptionDatacenterKey,
		Description:    "Identifier of the datacenter this Hub is hosted in.",
		Order:          publicCfgOptionDatacenterOrder,
		OptType:        config.OptTypeString,
		ExpertiseLevel: config.ExpertiseLevelExpert,
		DefaultValue:   publicCfgOptionDatacenterDefault,
	})
	if err != nil {
		return err
	}
	publicCfgOptionDatacenter = config.GetAsString(publicCfgOptionDatacenterKey, publicCfgOptionDatacenterDefault)

	err = config.Register(&config.Option{
		Name:           "IPv4",
		Key:            publicCfgOptionIPv4Key,
		Description:    "IPv4 address of this Hub. Must be globally reachable.",
		Order:          publicCfgOptionIPv4Order,
		OptType:        config.OptTypeString,
		ExpertiseLevel: config.ExpertiseLevelExpert,
		DefaultValue:   publicCfgOptionIPv4Default,
	})
	if err != nil {
		return err
	}
	publicCfgOptionIPv4 = config.GetAsString(publicCfgOptionIPv4Key, publicCfgOptionIPv4Default)

	err = config.Register(&config.Option{
		Name:           "IPv6",
		Key:            publicCfgOptionIPv6Key,
		Description:    "IPv6 address of this Hub. Must be globally reachable.",
		Order:          publicCfgOptionIPv6Order,
		OptType:        config.OptTypeString,
		ExpertiseLevel: config.ExpertiseLevelExpert,
		DefaultValue:   publicCfgOptionIPv6Default,
	})
	if err != nil {
		return err
	}
	publicCfgOptionIPv6 = config.GetAsString(publicCfgOptionIPv6Key, publicCfgOptionIPv6Default)

	err = config.Register(&config.Option{
		Name:           "Transports",
		Key:            publicCfgOptionTransportsKey,
		Description:    "List of transports this Hub supports.",
		Order:          publicCfgOptionTransportsOrder,
		OptType:        config.OptTypeStringArray,
		ExpertiseLevel: config.ExpertiseLevelExpert,
		DefaultValue:   publicCfgOptionTransportsDefault,
	})
	if err != nil {
		return err
	}
	publicCfgOptionTransports = config.GetAsStringArray(publicCfgOptionTransportsKey, publicCfgOptionTransportsDefault)

	err = config.Register(&config.Option{
		Name:           "Entry",
		Key:            publicCfgOptionEntryKey,
		Description:    "Define an entry policy. The format is the same for the endpoint lists. Default is permit.",
		Order:          publicCfgOptionEntryOrder,
		OptType:        config.OptTypeStringArray,
		ExpertiseLevel: config.ExpertiseLevelExpert,
		DefaultValue:   publicCfgOptionEntryDefault,
	})
	if err != nil {
		return err
	}
	publicCfgOptionEntry = config.GetAsStringArray(publicCfgOptionEntryKey, publicCfgOptionEntryDefault)

	err = config.Register(&config.Option{
		Name:           "Exit",
		Key:            publicCfgOptionExitKey,
		Description:    "Define an exit policy. The format is the same for the endpoint lists. Default is permit.",
		Order:          publicCfgOptionExitOrder,
		OptType:        config.OptTypeStringArray,
		ExpertiseLevel: config.ExpertiseLevelExpert,
		DefaultValue:   publicCfgOptionExitDefault,
	})
	if err != nil {
		return err
	}
	publicCfgOptionExit = config.GetAsStringArray(publicCfgOptionExitKey, publicCfgOptionExitDefault)

	// update defaults from system
	setDynamicPublicDefaults()

	return nil
}

func getPublicHubInfo() *hub.HubAnnouncement {
	// get configuration
	info := &hub.HubAnnouncement{
		Name:           publicCfgOptionName(),
		Group:          publicCfgOptionGroup(),
		ContactAddress: publicCfgOptionContactAddress(),
		ContactService: publicCfgOptionContactService(),
		Hosters:        publicCfgOptionHosters(),
		Datacenter:     publicCfgOptionDatacenter(),
		Transports:     publicCfgOptionTransports(),
		Entry:          publicCfgOptionEntry(),
		Exit:           publicCfgOptionExit(),
	}

	ip4 := publicCfgOptionIPv4()
	if ip4 != "" {
		ip := net.ParseIP(ip4)
		if ip == nil {
			log.Warningf("spn/cabin: invalid %s config: %s", publicCfgOptionIPv4Key, ip4)
		} else {
			info.IPv4 = ip
		}
	}

	ip6 := publicCfgOptionIPv6()
	if ip6 != "" {
		ip := net.ParseIP(ip6)
		if ip == nil {
			log.Warningf("spn/cabin: invalid %s config: %s", publicCfgOptionIPv6Key, ip6)
		} else {
			info.IPv6 = ip
		}
	}

	return info
}

func setDynamicPublicDefaults() {
	// name
	hostname, err := os.Hostname()
	if err == nil {
		err := config.SetDefaultConfigOption(publicCfgOptionNameKey, hostname)
		if err != nil {
			log.Warningf("spn/cabin: failed to set %s default to %s", publicCfgOptionNameKey, hostname)
		}
	}

	// IPs
	v4IPs, v6IPs, err := netenv.GetAssignedGlobalAddresses()
	if err != nil {
		log.Warningf("spn/cabin: failed to get assigned addresses: %s", err)
		return
	}
	if len(v4IPs) == 1 {
		err = config.SetDefaultConfigOption(publicCfgOptionIPv4Key, v4IPs[0].String())
		if err != nil {
			log.Warningf("spn/cabin: failed to set %s default to %s", publicCfgOptionIPv4Key, v4IPs[0].String())
		}
	}
	if len(v6IPs) == 1 {
		err = config.SetDefaultConfigOption(publicCfgOptionIPv6Key, v6IPs[0].String())
		if err != nil {
			log.Warningf("spn/cabin: failed to set %s default to %s", publicCfgOptionIPv6Key, v6IPs[0].String())
		}
	}
}

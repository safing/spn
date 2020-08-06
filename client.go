package main

import (
	"net"
	"os"
	"time"

	"github.com/safing/portbase/config"
	"github.com/safing/portbase/info"
	"github.com/safing/portbase/log"
	"github.com/safing/portbase/run"
	"github.com/safing/portmaster/network/packet"
	"github.com/safing/spn/captain"
	"github.com/safing/spn/conf"
	"github.com/safing/spn/sluice"

	// include packages here
	_ "github.com/safing/portmaster/core/base"
)

func main() {
	// configure
	info.Set("SPN Hub", "0.2.0", "AGPLv3", true)
	conf.EnablePublicHub(false)
	conf.EnableClient(true)
	config.SetDefaultConfigOption(captain.CfgOptionEnableSPNKey, true)

	go clientSim()

	// start
	os.Exit(run.Run())
}

func clientSim() {
	time.Sleep(10 * time.Second)

	log.Warning("starting to setup integration test environment")

	ips, err := net.LookupIP("detectportal.firefox.com")
	if err != nil {
		log.Errorf("failed to resolve: %s", err)
	}

	var i uint16
	for i = 1025; i < 65535; i++ {
		err := sluice.AwaitRequest(
			&packet.Info{
				Inbound:  false,
				Version:  packet.IPv4,
				Protocol: packet.TCP,
				SrcPort:  i,
				DstPort:  80,
				Dst:      ips[0],
			},
			"detectportal.firefox.com.",
		)
		if err != nil {
			log.Errorf("failed to inject request: %s", err)
			return
		}
	}

	log.Warningf("test environment finished")
	log.Warningf("use: curl --connect-to ::127.0.0.17:717 detectportal.firefox.com -vvv")
}

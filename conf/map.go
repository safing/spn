package conf

import (
	"flag"

	"github.com/safing/spn/hub"
)

var (
	MainMapName  = "main"
	MainMapScope = hub.ScopePublic
)

func init() {
	flag.StringVar(&MainMapName, "spn-map", "main", "set main SPN map - use only for testing")
}

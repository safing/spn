package identity

import (
	"flag"

	"github.com/safing/portbase/modules"
)

var (
	nodeName string

	module *modules.Module
)

func init() {
	flag.StringVar(&nodeName, "name", "", "name of node")

	module = modules.Register("identitymgr", nil, start, nil, "base")
}

func start() error {
	initIdentity()

	go manager()

	return nil
}

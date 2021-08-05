module github.com/safing/spn

go 1.15

require (
	github.com/brianvoe/gofakeit v3.18.0+incompatible
	github.com/davecgh/go-spew v1.1.1
	github.com/mr-tron/base58 v1.2.0
	github.com/safing/jess v0.2.2
	github.com/safing/portbase v0.11.0
	github.com/safing/portmaster v0.6.18
	github.com/stretchr/testify v1.6.1
	github.com/tevino/abool v1.2.0
	github.com/xtaci/kcp-go/v5 v5.6.1
)

replace (
	// Portbase
	github.com/safing/portbase => ../portbase
	github.com/safing/portbase/api => ../portbase/api
	github.com/safing/portbase/api/client => ../portbase/api/client
	github.com/safing/portbase/api/testclient => ../portbase/api/testclient
	github.com/safing/portbase/config => ../portbase/config
	github.com/safing/portbase/container => ../portbase/container
	github.com/safing/portbase/database => ../portbase/database
	github.com/safing/portbase/database/accessor => ../portbase/database/accessor
	github.com/safing/portbase/database/dbmodule => ../portbase/database/dbmodule
	github.com/safing/portbase/database/iterator => ../portbase/database/iterator
	github.com/safing/portbase/database/query => ../portbase/database/query
	github.com/safing/portbase/database/record => ../portbase/database/record
	github.com/safing/portbase/database/storage => ../portbase/database/storage
	github.com/safing/portbase/database/storage/badger => ../portbase/database/storage/badger
	github.com/safing/portbase/database/storage/bbolt => ../portbase/database/storage/bbolt
	github.com/safing/portbase/database/storage/fstree => ../portbase/database/storage/fstree
	github.com/safing/portbase/database/storage/hashmap => ../portbase/database/storage/hashmap
	github.com/safing/portbase/database/storage/sinkhole => ../portbase/database/storage/sinkhole
	github.com/safing/portbase/dataroot => ../portbase/dataroot
	github.com/safing/portbase/formats/dsd => ../portbase/formats/dsd
	github.com/safing/portbase/formats/varint => ../portbase/formats/varint
	github.com/safing/portbase/info => ../portbase/info
	github.com/safing/portbase/info/module => ../portbase/info/module
	github.com/safing/portbase/log => ../portbase/log
	github.com/safing/portbase/metrics => ../portbase/metrics
	github.com/safing/portbase/modules => ../portbase/modules
	github.com/safing/portbase/modules/subsystems => ../portbase/modules/subsystems
	github.com/safing/portbase/notifications => ../portbase/notifications
	github.com/safing/portbase/rng => ../portbase/rng
	github.com/safing/portbase/rng/test => ../portbase/rng/test
	github.com/safing/portbase/run => ../portbase/run
	github.com/safing/portbase/runtime => ../portbase/runtime
	github.com/safing/portbase/template => ../portbase/template
	github.com/safing/portbase/updater => ../portbase/updater
	github.com/safing/portbase/updater/uptool => ../portbase/updater/uptool
	github.com/safing/portbase/utils => ../portbase/utils
	github.com/safing/portbase/utils/debug => ../portbase/utils/debug
	github.com/safing/portbase/utils/osdetail => ../portbase/utils/osdetail

	// Portmaster
	github.com/safing/portmaster/cmds/portmaster-core => ../portmaster/cmds/portmaster-core
	github.com/safing/portmaster/cmds/portmaster-start => ../portmaster/cmds/portmaster-start
	github.com/safing/portmaster/cmds/trafficgen => ../portmaster/cmds/trafficgen
	github.com/safing/portmaster/cmds/updatemgr => ../portmaster/cmds/updatemgr
	github.com/safing/portmaster/core => ../portmaster/core
	github.com/safing/portmaster/core/base => ../portmaster/core/base
	github.com/safing/portmaster/core/pmtesting => ../portmaster/core/pmtesting
	github.com/safing/portmaster/detection/dga => ../portmaster/detection/dga
	github.com/safing/portmaster/firewall => ../portmaster/firewall
	github.com/safing/portmaster/firewall/inspection => ../portmaster/firewall/inspection
	github.com/safing/portmaster/firewall/interception => ../portmaster/firewall/interception
	github.com/safing/portmaster/firewall/interception/nfq => ../portmaster/firewall/interception/nfq
	github.com/safing/portmaster/intel => ../portmaster/intel
	github.com/safing/portmaster/intel/filterlists => ../portmaster/intel/filterlists
	github.com/safing/portmaster/intel/geoip => ../portmaster/intel/geoip
	github.com/safing/portmaster/nameserver => ../portmaster/nameserver
	github.com/safing/portmaster/nameserver/nsutil => ../portmaster/nameserver/nsutil
	github.com/safing/portmaster/netenv => ../portmaster/netenv
	github.com/safing/portmaster/network => ../portmaster/network
	github.com/safing/portmaster/network/netutils => ../portmaster/network/netutils
	github.com/safing/portmaster/network/packet => ../portmaster/network/packet
	github.com/safing/portmaster/network/proc => ../portmaster/network/proc
	github.com/safing/portmaster/network/reference => ../portmaster/network/reference
	github.com/safing/portmaster/network/socket => ../portmaster/network/socket
	github.com/safing/portmaster/network/state => ../portmaster/network/state
	github.com/safing/portmaster/process => ../portmaster/process
	github.com/safing/portmaster/profile => ../portmaster/profile
	github.com/safing/portmaster/profile/endpoints => ../portmaster/profile/endpoints
	github.com/safing/portmaster/profile/fingerprint => ../portmaster/profile/fingerprint
	github.com/safing/portmaster/resolver => ../portmaster/resolver
	github.com/safing/portmaster/status => ../portmaster/status
	github.com/safing/portmaster/ui => ../portmaster/ui
	github.com/safing/portmaster/updates => ../portmaster/updates

	// SPN
	github.com/safing/spn/access => ../spn/access
	github.com/safing/spn/api => ../spn/api
	github.com/safing/spn/cabin => ../spn/cabin
	github.com/safing/spn/captain => ../spn/captain
	github.com/safing/spn/cmds/clientsim => ../spn/cmds/clientsim
	github.com/safing/spn/cmds/hub => ../spn/cmds/hub
	github.com/safing/spn/conf => ../spn/conf
	github.com/safing/spn/docks => ../spn/docks
	github.com/safing/spn/hub => ../spn/hub
	github.com/safing/spn/navigator => ../spn/navigator
	github.com/safing/spn/navigator/dijkstra => ../spn/navigator/dijkstra
	github.com/safing/spn/ships => ../spn/ships
	github.com/safing/spn/sluice => ../spn/sluice

	// Jess
	github.com/safing/jess => ../jess
	github.com/safing/jess/hashtools => ../jess/hashtools
	github.com/safing/jess/lhash => ../jess/lhash
	github.com/safing/jess/tools => ../jess/tools
	github.com/safing/jess/tools/all => ../jess/tools/all
	github.com/safing/jess/tools/ecdh => ../jess/tools/ecdh
	github.com/safing/jess/tools/gostdlib => ../jess/tools/gostdlib
)

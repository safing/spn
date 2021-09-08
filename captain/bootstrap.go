package captain

import (
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"net"
	"os"

	"github.com/safing/jess/lhash"
	"github.com/safing/portbase/database"
	"github.com/safing/portbase/formats/dsd"
	"github.com/safing/portbase/log"
	"github.com/safing/portmaster/updates"
	"github.com/safing/spn/conf"
	"github.com/safing/spn/hub"
	"github.com/safing/spn/navigator"
)

type BootstrapFile struct {
	PublicHubs []string
}

var (
	bootstrapHubFlag  string
	bootstrapFileFlag string
)

func init() {
	flag.StringVar(&bootstrapHubFlag, "bootstrap-hub", "", "transport address of hub for bootstrapping with the hub ID in the fragment")
	flag.StringVar(&bootstrapFileFlag, "bootstrap-file", "", "bootstrap file containing bootstrap hubs - will be initialized if running a public hub and it doesn't exist")
}

// prepBootstrapHubFlag checks the bootstrap-hub argument if it is valid.
func prepBootstrapHubFlag() error {
	if bootstrapHubFlag != "" {
		return processBootstrapHub(bootstrapHubFlag, false)
	}
	return nil
}

// processBootstrapHubFlag processes the bootstrap-hub argument.
func processBootstrapHubFlag() error {
	if bootstrapHubFlag != "" {
		return processBootstrapHub(bootstrapHubFlag, true)
	}
	return nil
}

// processBootstrapHub processes the bootstrap-hub argument.
func processBootstrapHub(bootstrapTransport string, save bool) error {
	// parse argument
	t, err := hub.ParseTransport(bootstrapTransport)
	if err != nil {
		return fmt.Errorf("invalid bootstrap hub: %s", err)
	}
	if t.Option == "" {
		return errors.New("missing hub ID in URL fragment")
	}
	if _, err := lhash.FromBase58(t.Option); err != nil {
		return fmt.Errorf("hub ID is invalid: %w", err)
	}
	ip := net.ParseIP(t.Domain)
	if ip == nil {
		return errors.New("invalid IP address (domains are not supported)")
	}

	// abort if we are not saving to database
	if !save {
		return nil
	}

	// check if hub already exists
	_, err = hub.GetHub(hub.ScopePublic, t.Option)
	if err == nil {
		return nil
	}
	if !errors.Is(err, database.ErrNotFound) {
		return err
	}

	// prepare transport
	id := t.Option
	t.Domain = ""
	t.Option = ""

	// bootstrap Hub
	bootstrapHub := &hub.Hub{
		ID:    id,
		Scope: hub.ScopePublic,
		Info: &hub.Announcement{
			ID:         id,
			Transports: []string{t.String()},
		},
		Status: &hub.Status{},
	}

	// set IP address
	if ip4 := ip.To4(); ip4 != nil {
		bootstrapHub.Info.IPv4 = ip4
	} else {
		bootstrapHub.Info.IPv6 = ip
	}

	// Add to map for bootstrapping.
	navigator.Main.UpdateHub(bootstrapHub)

	log.Infof("spn/captain: added bootstrap %s", bootstrapHub)
	return nil
}

// processBootstrapFileFlag processes the bootstrap-file argument.
func processBootstrapFileFlag() error {
	if bootstrapFileFlag == "" {
		return nil
	}

	_, err := os.Stat(bootstrapFileFlag)
	if err != nil {
		if os.IsNotExist(err) {
			return createBootstrapFile(bootstrapFileFlag)
		} else {
			return fmt.Errorf("failed to access bootstrap hub file: %w", err)
		}
	}

	return loadBootstrapFile(bootstrapFileFlag)
}

// bootstrapWithUpdates loads bootstrap hubs from the updates server and imports them.
func bootstrapWithUpdates() error {
	if bootstrapFileFlag != "" {
		return errors.New("using the bootstrap-file argument disables bootstrapping via the update system")
	}

	file, err := updates.GetFile("spn/bootstrap.dsd")
	if err != nil {
		return fmt.Errorf("failed to get updates file: %w", err)
	}

	return loadBootstrapFile(file.Path())
}

// loadBootstrapFile loads a file with bootstrap hub entries and imports them.
func loadBootstrapFile(filename string) (err error) {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("failed to load bootstrap file: %w", err)
	}

	bs := &BootstrapFile{}
	_, err = dsd.Load(data, bs)
	if err != nil {
		return fmt.Errorf("failed to parse bootstrap file: %w", err)
	}

	var firstErr error
	for _, bsHub := range bs.PublicHubs {
		err = processBootstrapHub(bsHub, true)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			log.Warningf("spn/captain: failed to load bootstrap hub %q: %s", bsHub, err)
		}
	}

	if firstErr == nil {
		log.Infof("spn/captain: loaded bootstrap file %s", filename)
	}
	return firstErr
}

// createBootstrapFile save a bootstrap hub file with an entry of the public identity.
func createBootstrapFile(filename string) error {
	if !conf.PublicHub() {
		log.Infof("spn/captain: skipped writing a bootstrap hub file, as this is not a public hub")
		return nil
	}

	// create bootstrap hub
	if len(publicIdentity.Hub.Info.Transports) == 0 {
		return errors.New("public identity has no transports available")
	}
	// parse first transport
	t, err := hub.ParseTransport(publicIdentity.Hub.Info.Transports[0])
	if err != nil {
		return fmt.Errorf("failed to parse transport of public identity: %w", err)
	}
	// add IP address
	if publicIdentity.Hub.Info.IPv4 != nil {
		t.Domain = publicIdentity.Hub.Info.IPv4.String()
	} else if publicIdentity.Hub.Info.IPv6 != nil {
		t.Domain = "[" + publicIdentity.Hub.Info.IPv6.String() + "]"
	} else {
		return errors.New("public identity has no IP address available")
	}
	// add Hub ID
	t.Option = publicIdentity.Hub.ID
	// put together
	bs := &BootstrapFile{
		PublicHubs: []string{t.String()},
	}

	// serialize
	fileData, err := dsd.Dump(bs, dsd.JSON)
	if err != nil {
		return err
	}

	// save to disk
	err = ioutil.WriteFile(filename, fileData, 0664)
	if err != nil {
		return err
	}

	log.Infof("spn/captain: created bootstrap file %s", filename)
	return nil
}

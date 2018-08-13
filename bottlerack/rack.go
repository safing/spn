package bottlerack

import (
	"fmt"
	"time"

	ds "github.com/ipfs/go-datastore"

	"github.com/Safing/safing-core/database"
	"github.com/Safing/safing-core/database/dbutils"
	"github.com/Safing/safing-core/log"
	"github.com/Safing/safing-core/port17/bottle"
)

var (
	DatabaseNamespace = ds.NewKey("/Bottles")
	publicBottles     = DatabaseNamespace.ChildString("Public")
	localBottles      = DatabaseNamespace.ChildString("Local")
)

func Get(portName string) *bottle.Bottle {
	b, err := loadPublicBottle(portName)
	if err != nil {
		return nil
	}
	return b
}

func loadPublicBottle(portName string) (*bottle.Bottle, error) {
	return LoadBottle(publicBottles.ChildString(portName))
}

func loadLocalBottle(portName string) (*bottle.Bottle, error) {
	return LoadBottle(publicBottles.ChildString(portName))
}

func LoadBottle(key ds.Key) (*bottle.Bottle, error) {
	wrapper, err := database.RawGet(key)
	if err != nil {
		if err == ds.ErrNotFound {
			return nil, err
		}
		return nil, fmt.Errorf("failed to get bottle from database: %s", err)
	}
	b, err := bottle.LoadTrustedBottle(wrapper.Data)
	if err != nil {
		return nil, fmt.Errorf("failed to load bottle: %s", err)
	}
	return b, nil
}

func SavePublicBottle(b *bottle.Bottle) {
	// TODO: add 1 month expiry
	SaveBottle(publicBottles.ChildString(b.PortName), b)
}

func SaveLocalBottle(b *bottle.Bottle) {
	// TODO: add 1 day expiry
	SaveBottle(localBottles.ChildString(b.PortName), b)
}

func SaveBottle(key ds.Key, b *bottle.Bottle) {
	b.LastUpdate = time.Now().Unix()
	err := database.RawPut(key, b)
	if err != nil {
		log.Warningf("port17 bottlerack: failed to save bottle %s", b.PortName)
	}
}

func DiscardPublicBottle(portName string) {
	err := database.Delete(publicBottles.ChildString(portName))
	if err != nil {
		log.Warningf("port17/bottlerack: failed to delete public Bottle %s", portName)
	}
}

// PublicBottleFeed returns a feed of all public bottles
func PublicBottleFeed() (chan *bottle.Bottle, error) {
	feed := make(chan *bottle.Bottle, 10)
	entries, err := database.EasyQuery("/Bottles/Public/*")
	if err != nil {
		close(feed)
		return feed, fmt.Errorf("port17/bottlerack: failed to get bottles for feeding the navigator: %s", err)
	}
	go func() {
		for _, entry := range *entries {
			wrapped, ok := entry.Value.(*dbutils.Wrapper)
			if ok {
				b, err := bottle.LoadTrustedBottle(wrapped.Data)
				if err != nil {
					log.Warningf("port17/bottlerack: failed to load bottle: %s", err)
					continue
				}
				feed <- b
			}
		}
		close(feed)
	}()
	return feed, nil
}

package bottle

import (
	"fmt"
	"time"

	"github.com/safing/tinker"
)

type BottleKey struct {
	Expires int64               `json:"e" bson:"e"`
	Key     *tinker.EphermalKey `json:"k" bson:"k"`
}

func (bk *BottleKey) String() string {
	switch {
	case bk.Key == nil:
		return "burnt"
	case bk.Expired():
		return fmt.Sprintf("%s*", bk.Key.Algorithm)
	default:
		return bk.Key.Algorithm
	}
}

func (bk *BottleKey) Expired() bool {
	return time.Now().Unix() > bk.Expires
}

func (bk *BottleKey) Public() *BottleKey {
	return &BottleKey{
		Expires: bk.Expires,
		Key:     bk.Key.Public(),
	}
}

func (b *Bottle) GetValidKey() (keyID int, validKey *BottleKey) {
	b.Lock()
	defer b.Unlock()
	keyID = -1
	for id, key := range b.Keys {
		if validKey == nil || validKey.Expires <= key.Expires {
			if !key.Expired() {
				keyID = id
				validKey = key
			}
		}
	}
	return
}

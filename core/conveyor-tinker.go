package core

import (
	"fmt"

	"github.com/safing/portbase/container"
	"github.com/safing/spn/bottle"
	"github.com/safing/tinker"
)

type TinkerConveyor struct {
	ConveyorBase
	tinker *tinker.Tinker
}

func NewTinkerConveyor(server bool, init *Initializer, serverBottle *bottle.Bottle) (*TinkerConveyor, error) {
	tk := tinker.NewTinker(tinker.RecommendedNetwork)
	if server {
		tk.CommServer().NoRemoteAuth()
	} else {
		tk.CommClient().NoSelfAuth()
	}

	var err error
	if serverBottle == nil {
		serverBottle, err = bottle.GetPublicBottle(init.DestPortName)
		if err != nil {
			return nil, fmt.Errorf("failed to get server bottle for tinker: %s", err)
		}
	}

	serverBottle.Lock()
	for _, supplyID := range init.KeyexIDs {
		tk.SupplyAuthenticatedServerExchKey(serverBottle.Keys[supplyID].Key)
	}
	serverBottle.Unlock()

	_, err = tk.Init()
	if err != nil {
		return nil, err
	}

	return &TinkerConveyor{tinker: tk}, nil
}

func (tc *TinkerConveyor) Run() {
	for {
		select {
		case c := <-tc.fromShore:

			// silent fail
			if c == nil {
				return
			}

			// encrypt (even if error)
			encrypted, err := tc.tinker.Encrypt(c.CompileData())
			if err != nil {
				c.SetError(err)
				tc.toShip <- c
				tc.toShore <- c
				return
			}

			// send on its way
			c = container.NewContainer(encrypted)
			tc.toShip <- c

		case c := <-tc.fromShip:

			// silent fail
			if c == nil {
				return
			}

			// forward if error
			if c.HasError() {
				tc.toShore <- c
				return
			}

			// decrypt
			decrypted, err := tc.tinker.Decrypt(c.CompileData())
			if err != nil {
				c.SetError(err)
				tc.toShip <- c
				tc.toShore <- c
				return
			}

			// check if error
			c = container.NewContainer(decrypted)
			c.CheckError()
			if c.HasError() {
				// close other direction silenty
				tc.toShip <- nil
			}

			// send on its way (data or error)
			tc.toShore <- c

		}
	}
}

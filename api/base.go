package api

import (
	"sync"

	"github.com/tevino/abool"

	"github.com/Safing/safing-core/container"
	"github.com/Safing/safing-core/formats/varint"
	"github.com/Safing/safing-core/log"
)

type APIBase struct {
	// implement LastConveyor
	lineID   string
	fromShip chan *container.Container
	toShip   chan *container.Container

	server    bool
	initiator bool
	nextID    uint32

	activeCalls     map[uint32]*Call
	activeCallsLock sync.Mutex
	abandoned       *abool.AtomicBool

	handlers map[uint8]ApiHandler
}

func (api *APIBase) Init(server, initiator bool, fromShip, toShip chan *container.Container) {
	api.server = server
	api.initiator = initiator
	api.fromShip = fromShip
	api.toShip = toShip

	api.activeCalls = make(map[uint32]*Call)
	api.handlers = make(map[uint8]ApiHandler)
	api.abandoned = abool.NewBool(false)

	if !api.initiator {
		api.nextID = 1
	}
}

func (api *APIBase) RegisterHandler(id uint8, handler ApiHandler) {
	api.handlers[id] = handler
}

func (api *APIBase) GetNextID() uint32 {
	for {
		if api.nextID > 2147483640 {
			api.nextID -= 2147483640
		}
		api.nextID += 2
		api.activeCallsLock.Lock()
		_, ok := api.activeCalls[api.nextID]
		api.activeCallsLock.Unlock()
		if !ok {
			return api.nextID
		}
	}
}

func (api *APIBase) Run() {
	for {
		c := <-api.fromShip

		// silent fail
		if c == nil {
			api.Shutdown()
			return
		}

		// handle error
		if c.HasError() {
			log.Warningf("api on %s: received error: %s", api.lineID, c.Error())
			api.Shutdown()
			return
		}

		// get ID
		id, err := c.GetNextN32()
		if err != nil {
			log.Warningf("api on %s: failed to unpack: %s", api.lineID, err)
			api.Shutdown()
			return
		}

		// get msg type
		msgType, err := c.GetNextN8()
		if err != nil {
			log.Warningf("api on %s: failed to unpack: %s", api.lineID, err)
			api.Shutdown()
			return
		}

		if msgType == API_CALL {

			// log.Debugf("api: received call %d with data %s", msgType, string(c.CompileData()))

			handlerID, err := c.GetNextN8()
			if err != nil {
				log.Debugf("api on %s: failed to unpack: %s", api.lineID, err)
				api.Shutdown()
				return
			}

			newCall := &Call{
				Api:       api,
				Initiator: false,
				ID:        id,
				Msgs:      make(chan *ApiMsg, 0),
				ended:     abool.NewBool(false),
			}
			api.activeCallsLock.Lock()
			api.activeCalls[newCall.ID] = newCall
			api.activeCallsLock.Unlock()
			handler, ok := api.handlers[handlerID]
			if ok {
				go handler(newCall, c)
			} else {
				newCall.SendError("no handler for this call registered")
				newCall.End()
			}

			continue
		}

		api.activeCallsLock.Lock()
		call, ok := api.activeCalls[id]
		api.activeCallsLock.Unlock()
		if ok {
			call.Msgs <- &ApiMsg{
				MsgType:   msgType,
				Container: c,
			}
		}

	}
}

func (api *APIBase) Call(handlerID uint8, c *container.Container) *Call {
	if api.abandoned.IsSet() {
		return nil
	}

	newCall := &Call{
		Api:       api,
		Initiator: true,
		ID:        api.GetNextID(),
		Msgs:      make(chan *ApiMsg, 0),
		ended:     abool.NewBool(false),
	}

	api.activeCallsLock.Lock()
	api.activeCalls[newCall.ID] = newCall
	api.activeCallsLock.Unlock()

	c.Prepend(varint.Pack8(handlerID))
	newCall.send(API_CALL, c)
	return newCall
}

func (api *APIBase) Send(id uint32, msgType uint8, c *container.Container) {
	if !api.abandoned.IsSet() {
		c.Prepend(varint.Pack8(msgType))
		c.Prepend(varint.Pack32(id))
		api.toShip <- c
	}
}

func (api *APIBase) Shutdown() {
	if api.abandoned.SetToIf(false, true) {
		api.activeCallsLock.Lock()
		defer api.activeCallsLock.Unlock()
		for _, activeCall := range api.activeCalls {
			activeCall.ended.Set()
			close(activeCall.Msgs)
		}
		api.activeCalls = make(map[uint32]*Call)
		close(api.toShip)
	}
}

func (api *APIBase) EndCall(id uint32) {
	api.activeCallsLock.Lock()
	defer api.activeCallsLock.Unlock()
	call, ok := api.activeCalls[id]
	if ok {
		close(call.Msgs)
	}
	delete(api.activeCalls, id)
}

// AttachConveyorBelts attaches the Conveyor to a line.
func (api *APIBase) AttachConveyorBelts(lineID string, fromShip, toShip chan *container.Container) {
	api.lineID = lineID
	api.fromShip = fromShip
	api.toShip = toShip
}

func (api *APIBase) IsAbandoned() bool {
	return api.abandoned.IsSet()
}

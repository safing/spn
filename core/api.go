package core

import (
	"fmt"

	"github.com/safing/portbase/container"
	"github.com/safing/portbase/formats/dsd"
	"github.com/safing/portbase/info"
	"github.com/safing/portbase/log"
	"github.com/safing/spn/api"
	"github.com/safing/spn/bottle"
)

// API provides the interface that nodes communicate with
type API struct {
	api.APIBase
}

// Message type constants
const (
	// Informational
	MsgTypeInfo  uint8 = 1
	MsgTypeLoad  uint8 = 2
	MsgTypeStats uint8 = 3

	// Tunneling
	MsgTypeHop    uint8 = 4
	MsgTypeTunnel uint8 = 5
	MsgTypePing   uint8 = 6

	// Diagnostics
	MsgTypeEcho      uint8 = 7
	MsgTypeSpeedtest uint8 = 8

	// Mgmt
	MsgTypeEstablishRoute uint8 = 9
	MsgTypeShutdown       uint8 = 10

	// Other APIs
	MsgTypePort17Admin uint8 = 11
	MsgTypePortAccess  uint8 = 12

	// Network Information
	MsgTypeBottleFeed uint8 = 18
)

// NewAPI returns a new Instance of the Port17 API.
func NewAPI(server, initiator bool) *API {
	portAPI := &API{}
	portAPI.Init(server, initiator, nil, nil)

	portAPI.RegisterHandler(MsgTypeInfo, portAPI.handleInfo)
	portAPI.RegisterHandler(MsgTypeHop, portAPI.handleHop)
	portAPI.RegisterHandler(MsgTypeEcho, portAPI.handleEcho)
	portAPI.RegisterHandler(MsgTypeTunnel, portAPI.handleTunnel)
	portAPI.RegisterHandler(MsgTypePing, portAPI.handlePing)
	portAPI.RegisterHandler(MsgTypePing, portAPI.handlePing)
	portAPI.RegisterHandler(MsgTypeBottleFeed, portAPI.handleBottleFeed)

	return portAPI
}

// Info calls and returns node information of the connected node.
func (portAPI *API) Info() (*info.Info, error) {
	call := portAPI.Call(MsgTypeInfo, container.New())
	msg := <-call.Msgs
	switch msg.MsgType {
	case api.API_DATA:
		info := &info.Info{}
		_, err := dsd.Load(msg.Container.CompileData(), info)
		if err != nil {
			return nil, fmt.Errorf("could not parse data: %s", err)
		}
		return info, nil
	case api.API_ERR:
		return nil, fmt.Errorf("failed to get node info: %s", api.ParseError(msg.Container))
	}
	return nil, fmt.Errorf("received unexpected data")
}

func (portAPI *API) handleInfo(call *api.Call, none *container.Container) {
	info := info.GetInfo()
	data, err := dsd.Dump(info, dsd.JSON)
	if err != nil {
		log.Warningf("spn/api: failed to pack info: %s", err)
		call.SendError("could not pack info")
	} else {
		call.SendData(container.NewContainer(data))
	}
	call.End()
}

// Hop creates a new connection from the connected node to another one and returns a new Port17 API instance for that node.
func (portAPI *API) Hop(init *Initializer, targetBottle *bottle.Bottle) (*API, error) {
	var err error

	if targetBottle != nil {
		init.DestPortName = targetBottle.PortName
	}

	data, err := init.Pack()
	if err != nil {
		return nil, err
	}

	// call
	call := portAPI.Call(MsgTypeHop, container.New(data))
	// create new api
	newPortAPI := NewAPI(false, true)
	// build conveyor
	conveyor := NewSimpleConveyorLine()
	if init.TinkerVersion > 0 {
		tk, err := NewTinkerConveyor(false, init, targetBottle)
		if err != nil {
			return nil, err
		}
		conveyor.AddConveyor(tk)
	}
	conveyor.AddLastConveyor(newPortAPI)

	// start handling data
	go portAPI.relay(call, conveyor)

	return newPortAPI, nil
}

func (portAPI *API) relay(call *api.Call, conveyor *SimpleConveyorLine) {
	for {
		select {
		case msg := <-call.Msgs:
			switch msg.MsgType {
			case api.API_DATA:
				conveyor.toShore <- msg.Container
			case api.API_ERR:
				log.Warningf("port17: relay: got downstream error: %s", api.ParseError(msg.Container))
				close(conveyor.toShore)
			default:
				close(conveyor.toShore)
			}
		case c := <-conveyor.fromShore:
			if c == nil {
				close(conveyor.toShore)
				call.End()
				return
			}
			if c.HasError() {
				close(conveyor.toShore)
				call.SendError(c.ErrString())
				call.End()
				return
			}
			call.SendData(c)
		}
	}
}

func (portAPI *API) handleHop(call *api.Call, c *container.Container) {
	init, err := UnpackInitializer(c.CompileData())
	if err != nil {
		call.SendError(err.Error())
		call.End()
		return
	}

	crane := GetAssignedCrane(init.DestPortName)
	if crane == nil {
		call.SendError(fmt.Sprintf("no route to %s", init.DestPortName))
		call.End()
		return
	}

	init.DestPortName = ""
	convLine, err := crane.Controller.NewLine(init)
	if err != nil {
		call.SendError(fmt.Sprintf("failed to create line: %s", err))
		call.End()
		return
	}

	StartPortRelay(call, convLine)
}

// Echo send the given data to the connected node and returns the received data.
func (portAPI *API) Echo(data []byte) ([]byte, error) {
	call := portAPI.Call(MsgTypeEcho, container.New(data))
	echo := <-call.Msgs
	switch echo.MsgType {
	case api.API_DATA:
		call.End()
		return echo.Container.CompileData(), nil
	case api.API_ERR:
		return nil, fmt.Errorf("failed to call echo: %s", api.ParseError(echo.Container))
	default:
		return nil, fmt.Errorf("unexpected reply: %d", echo.MsgType)
	}
}

func (portAPI *API) handleEcho(call *api.Call, c *container.Container) {
	data := c.CompileData()
	for i, j := 0, len(data)-1; i < j; i, j = i+1, j-1 {
		data[i], data[j] = data[j], data[i]
	}
	call.SendData(container.New(data))
	call.End()
}

func (portAPI *API) handlePing(call *api.Call, c *container.Container) {
	call.SendAck()
}

// NewClient is used to create the initial hop to a node and returns a new Port17 API.
func NewClient(init *Initializer, targetBottle *bottle.Bottle) (*API, error) {

	var err error

	if targetBottle == nil {
		targetBottle, err = bottle.Get(init.DestPortName)
		if err != nil {
			return nil, fmt.Errorf("failed to get destination bottle: %w", err)
		}
	}
	init.DestPortName = ""
	keyID, _ := targetBottle.GetValidKey()
	init.KeyexIDs = []int{keyID}

	crane := GetAssignedCrane(targetBottle.PortName)
	if crane == nil {
		return nil, fmt.Errorf("port17: no route to %s", targetBottle.PortName)
	}

	line, err := crane.Controller.NewLine(init)
	if err != nil {
		return nil, fmt.Errorf("port17: failed to create line: %s", err)
	}

	// create new api
	newPortAPI := NewAPI(false, true)

	// build line
	if init.TinkerVersion > 0 {
		tk, err := NewTinkerConveyor(false, init, targetBottle)
		if err != nil {
			return nil, err
		}
		line.AddConveyor(tk)
	}
	line.AddLastConveyor(newPortAPI)

	return newPortAPI, nil
}

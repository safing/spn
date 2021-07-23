package docks

/*
import (
	"fmt"
	"time"

	"github.com/safing/portbase/container"
	"github.com/safing/portbase/formats/dsd"
	"github.com/safing/portbase/info"
	"github.com/safing/portbase/log"
	"github.com/safing/spn/access"
	"github.com/safing/spn/api"
	"github.com/safing/spn/hub"
)

// API provides the interface that nodes communicate with
type API struct {
	api.APIBase
}

// Message type constants
const (
	// Informational
	MsgTypeInfo          uint8 = 1
	MsgTypeLoad          uint8 = 2
	MsgTypeStats         uint8 = 3
	MsgTypePublicHubFeed uint8 = 4

	// Diagnostics
	MsgTypeEcho      uint8 = 16
	MsgTypeSpeedtest uint8 = 17

	// User Access
	MsgTypeUserAuth uint8 = 32

	// Tunneling
	MsgTypeHop    uint8 = 40
	MsgTypeTunnel uint8 = 41
	MsgTypePing   uint8 = 42

	// Admin/Mod Access
	MsgTypeAdminAuth uint8 = 64

	// Mgmt
	MsgTypeEstablishRoute uint8 = 72
	MsgTypeShutdown       uint8 = 73
)

// NewAPI returns a new Instance of the Port17 API.
func NewAPI(version int, server, initiator bool) *API {
	portAPI := &API{}
	portAPI.Init(server, initiator, nil, nil)

	portAPI.RegisterHandler(MsgTypeUserAuth, portAPI.handleUserAuth)

	portAPI.RegisterHandler(MsgTypeInfo, portAPI.handleInfo)
	portAPI.RegisterHandler(MsgTypeEcho, portAPI.handleEcho)
	portAPI.RegisterHandler(MsgTypePublicHubFeed, portAPI.handlePublicHubFeed)

	return portAPI
}

// Info calls and returns node information of the connected node.
func (portAPI *API) UserAuth(code *access.Code) error {
	call := portAPI.Call(MsgTypeUserAuth, container.New(code.Raw()))
	select {
	case msg := <-call.Msgs:
		switch msg.MsgType {
		case api.API_ACK:
			return nil
		case api.API_ERR:
			return fmt.Errorf("failed authenticate with access code: %s", api.ParseError(msg.Container))
		}
		return fmt.Errorf("received unexpected data")
	case <-time.After(1 * time.Second):
		return fmt.Errorf("timed out")
	}
}

func (portAPI *API) handleUserAuth(call *api.Call, data *container.Container) {
	// parse code
	code, err := access.ParseRawCode(data.CompileData())
	if err != nil {
		call.SendError(fmt.Sprintf("failed to parse access code: %s", err))
		call.End()
		return
	}

	// verify code
	err = access.Check(code)
	if err != nil {
		call.SendError(fmt.Sprintf("access code verification failed: %s", err))
		call.End()
		return
	}

	// register additional handlers
	portAPI.RegisterHandler(MsgTypeHop, portAPI.handleHop)
	portAPI.RegisterHandler(MsgTypeTunnel, portAPI.handleTunnel)
	portAPI.RegisterHandler(MsgTypePing, portAPI.handlePing)
	call.SendAck()
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
func (portAPI *API) Hop(version int, dst *hub.Hub) (*API, error) {
	// build request
	msg := container.New()
	msg.AppendNumber(uint64(version))
	msg.AppendAsBlock([]byte(dst.ID))

	// call
	call := portAPI.Call(MsgTypeHop, msg)
	// build conveyor
	conveyor := NewSimpleConveyorLine()

	// add encryption
	ec, err := NewEncryptionConveyor(version, nil, dst)
	if err != nil {
		return nil, err
	}
	conveyor.AddConveyor(ec)

	// add API
	newPortAPI := NewAPI(version, false, true)
	conveyor.AddLastConveyor(newPortAPI)

	// start handling data
	go portAPI.relay(call, conveyor)

	return newPortAPI, nil
}

func (portAPI *API) relay(call *api.Call, conveyor *SimpleConveyorLine) {
	for {
		select {
		case msg := <-call.Msgs:
			if msg == nil { // call ended
				close(conveyor.toShore)
				return
			}
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
	version, err := c.GetNextN8()
	if err != nil {
		call.SendError(err.Error())
		call.End()
		return
	}

	hubID, err := c.GetNextBlock()
	if err != nil {
		call.SendError(err.Error())
		call.End()
		return
	}

	crane := GetAssignedCrane(string(hubID))
	if crane == nil {
		call.SendError(fmt.Sprintf("no route to %s", string(hubID)))
		call.End()
		return
	}

	convLine, err := crane.Controller.NewLine(int(version))
	if err != nil {
		call.SendError(fmt.Sprintf("failed to create line: %s", err))
		call.End()
		return
	}

	relay := &HubRelay{
		call: call,
	}
	convLine.AddLastConveyor(relay)
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
func NewClient(version int, dst *hub.Hub) (*API, error) {
	crane := GetAssignedCrane(dst.ID)
	if crane == nil {
		return nil, fmt.Errorf("no crane to %s", dst.ID)
	}

	line, err := crane.Controller.NewLine(version)
	if err != nil {
		return nil, fmt.Errorf("failed to create line: %w", err)
	}

	// create new api
	newPortAPI := NewAPI(version, false, true)

	// build line
	ce, err := NewEncryptionConveyor(version, nil, dst)
	if err != nil {
		return nil, err
	}
	line.AddConveyor(ce)
	line.AddLastConveyor(newPortAPI)

	return newPortAPI, nil
}
*/

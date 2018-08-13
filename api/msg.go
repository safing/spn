package api

import "github.com/Safing/safing-core/container"

type ApiMsg struct {
	MsgType   uint8
	Container *container.Container
}

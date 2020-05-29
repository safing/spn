package api

import "github.com/safing/portbase/container"

type ApiMsg struct {
	MsgType   uint8
	Container *container.Container
}

package terminal

import "github.com/safing/portbase/container"

//
type AssignmentFactory func(t *Terminal, assignmentID uint32, initialData *container.Container) Assignment

type Assignment interface {
	Create(t *Terminal, id uint32, initialData *container.Container)
	Deliver(data *container.Container)
	ReportError(err error)
	Done()
}

type AssignmentBase struct {
	ID uint32
}

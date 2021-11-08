package captain

import (
	"sync"
	"time"

	"github.com/safing/portbase/database/record"
	"github.com/safing/portbase/runtime"
)

type SPNStatus struct {
	record.Base
	sync.Mutex

	Status             SPNStatusName
	HomeHubID          string
	ConnectedIP        string
	ConnectedTransport string
	ConnectedSince     *time.Time
}

type SPNStatusName string

const (
	StatusFailed     SPNStatusName = "failed"
	StatusDisabled   SPNStatusName = "disabled"
	StatusConnecting SPNStatusName = "connecting"
	StatusConnected  SPNStatusName = "connected"
)

var (
	spnStatus = &SPNStatus{
		Status: StatusDisabled,
	}
	spnStatusPushFunc runtime.PushFunc
)

func registerSPNStatusProvider() (err error) {
	spnStatus.SetKey("runtime:spn/status")
	spnStatus.UpdateMeta()
	spnStatusPushFunc, err = runtime.Register("spn/status", runtime.ProvideRecord(spnStatus))
	return
}

func resetSPNStatus(statusName SPNStatusName) {
	// Lock for updating values.
	spnStatus.Lock()
	defer spnStatus.Unlock()

	// Reset status.
	spnStatus.Status = statusName
	spnStatus.HomeHubID = ""
	spnStatus.ConnectedIP = ""
	spnStatus.ConnectedTransport = ""
	spnStatus.ConnectedSince = nil

	// Push new status.
	pushSPNStatusUpdate()
}

// pushSPNStatusUpdate pushes an update of spnStatus, which must be locked.
func pushSPNStatusUpdate() {
	spnStatus.UpdateMeta()
	spnStatusPushFunc(spnStatus)
}

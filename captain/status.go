package captain

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/safing/portbase/config"
	"github.com/safing/portbase/database/record"
	"github.com/safing/portbase/runtime"
	"github.com/safing/portbase/utils/debug"
	"github.com/safing/spn/conf"
	"github.com/safing/spn/navigator"
)

// SPNStatus holds SPN status information.
type SPNStatus struct {
	record.Base
	sync.Mutex

	Status             SPNStatusName
	HomeHubID          string
	HomeHubName        string
	ConnectedIP        string
	ConnectedTransport string
	ConnectedSince     *time.Time
}

// SPNStatusName is a SPN status.
type SPNStatusName string

// SPN Stati.
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

func resetSPNStatus(statusName SPNStatusName, overrideEvenIfConnected bool) {
	// Lock for updating values.
	spnStatus.Lock()
	defer spnStatus.Unlock()

	// Ignore when connected and not overriding
	if !overrideEvenIfConnected && spnStatus.Status == StatusConnected {
		return
	}

	// Reset status.
	spnStatus.Status = statusName
	spnStatus.HomeHubID = ""
	spnStatus.HomeHubName = ""
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

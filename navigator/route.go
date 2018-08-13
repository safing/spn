package navigator

import (
	"fmt"
)

type Route struct {
	Port *Port
	Cost int
}

func (r *Route) String() string {
	return fmt.Sprintf("%s:%d", r.Port.Name(), r.Cost)
}

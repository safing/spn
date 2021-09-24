package navigator

import (
	"fmt"
	"strings"
)

func printPortPath(path []*Pin) string {
	s := ""
	for _, entry := range path {
		s += fmt.Sprintf("%s-", entry.Hub.Name())
	}
	return strings.Trim(s, "-")
}

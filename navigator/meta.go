package navigator

import (
	"fmt"
	"strings"
)

func printPortPath(path []*Port) string {
	s := ""
	for _, entry := range path {
		s += fmt.Sprintf("%s-", entry.Name())
	}
	return strings.Trim(s, "-")
}

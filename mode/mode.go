package mode

import (
	"github.com/tevino/abool"
)

var (
	isClient = abool.NewBool(false)
	isNode   = abool.NewBool(false)
)

func Node() bool {
	return isNode.IsSet()
}

func Client() bool {
	return isClient.IsSet()
}

func SetNode(on bool) {
	isNode.SetTo(on)
}

func SetClient(on bool) {
	isClient.SetTo(on)
}

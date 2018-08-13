package dijkstra

import (
	"sync"
	"testing"
)

func TestAlgorithm(t *testing.T) {
	collection := BuildTestNet()
	var collectionLock sync.Mutex

	dijkstra := New(map[string]Element(collection), &collectionLock, []byte{0x01})

}

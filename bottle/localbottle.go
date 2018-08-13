package bottle

type LocalBottle struct {
	MaskedIdentifier []byte `json:"mid,omitempty" bson:"mid,omitempty"`
	ReachableFrom    []byte `json:"at,omitempty" bson:"at,omitempty"`
}

package bottle

type BottleConnection struct {
	PortName string `json:"n" bson:"n"`
	Cost     int    `json:"c" bson:"c"` // measured in microseconds
}

func (bc *BottleConnection) String() string {
	return bc.PortName
}

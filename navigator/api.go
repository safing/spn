package navigator

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"text/tabwriter"
	"time"

	"github.com/awalterschulze/gographviz"
	"github.com/safing/portbase/api"
	"github.com/safing/portbase/log"
	"github.com/safing/spn/hub"
)

var (
	apiMapsLock sync.Mutex
	apiMaps     = make(map[string]*Map)
)

func addMapToAPI(m *Map) {
	apiMapsLock.Lock()
	defer apiMapsLock.Unlock()

	apiMaps[m.Name] = m
}

func getMapForAPI(name string) (m *Map, ok bool) {
	apiMapsLock.Lock()
	defer apiMapsLock.Unlock()

	m, ok = apiMaps[name]
	return
}

func removeMapFromAPI(name string) {
	apiMapsLock.Lock()
	defer apiMapsLock.Unlock()

	delete(apiMaps, name)
}

func registerAPIEndpoints() error {
	if err := api.RegisterEndpoint(api.Endpoint{
		Path:        `spn/map/{map:[A-Za-z0-9]{1,255}}/pins`,
		Read:        api.PermitUser,
		BelongsTo:   module,
		StructFunc:  handleMapPinsRequest,
		Name:        "Get SPN map pins",
		Description: "Returns a list of pins on the map.",
	}); err != nil {
		return err
	}

	if err := api.RegisterEndpoint(api.Endpoint{
		Path:        `spn/map/{map:[A-Za-z0-9]{1,255}}/optimization`,
		Read:        api.PermitUser,
		BelongsTo:   module,
		StructFunc:  handleMapOptimizationRequest,
		Name:        "Get SPN map optimization",
		Description: "Returns the calculated optimization for the map.",
	}); err != nil {
		return err
	}

	if err := api.RegisterEndpoint(api.Endpoint{
		Path:        `spn/map/{map:[A-Za-z0-9]{1,255}}/measurements`,
		Read:        api.PermitUser,
		BelongsTo:   module,
		StructFunc:  handleMapMeasurementsRequest,
		Name:        "Get SPN map measurements",
		Description: "Returns the measurements of the map.",
	}); err != nil {
		return err
	}

	if err := api.RegisterEndpoint(api.Endpoint{
		Path:        `spn/map/{map:[A-Za-z0-9]{1,255}}/measurements/table`,
		MimeType:    api.MimeTypeText,
		Read:        api.PermitUser,
		BelongsTo:   module,
		DataFunc:    handleMapMeasurementsTableRequest,
		Name:        "Get SPN map measurements as a table",
		Description: "Returns the measurements of the map as a table.",
	}); err != nil {
		return err
	}

	if err := api.RegisterEndpoint(api.Endpoint{
		Path:        `spn/map/{map:[A-Za-z0-9]{1,255}}/graph{format:\.[a-z]{2,4}}`,
		Read:        api.PermitUser,
		BelongsTo:   module,
		HandlerFunc: handleMapGraphRequest,
		Name:        "Get SPN map graph",
		Description: "Returns a graph of the given SPN map.",
		Parameters: []api.Parameter{
			{
				Method:      http.MethodGet,
				Field:       "map (in path)",
				Value:       "name of map",
				Description: "Specify the map you want to get the map for. The main map is called `main`.",
			},
			{
				Method:      http.MethodGet,
				Field:       "format (in path)",
				Value:       "file type",
				Description: "Specify the format you want to get the map in. Available values: `dot`, `html`. Please note that the html format is only available in development mode.",
			},
		},
	}); err != nil {
		return err
	}

	return nil
}

func handleMapPinsRequest(ar *api.Request) (i interface{}, err error) {
	// Get map.
	m, ok := getMapForAPI(ar.URLVars["map"])
	if !ok {
		return nil, errors.New("map not found")
	}

	// Export all pins.
	sortedPins := m.sortedPins(true)
	exportedPins := make([]*PinExport, len(sortedPins))
	for key, pin := range sortedPins {
		exportedPins[key] = pin.Export()
	}

	return exportedPins, nil
}

func handleMapOptimizationRequest(ar *api.Request) (i interface{}, err error) {
	// Get map.
	m, ok := getMapForAPI(ar.URLVars["map"])
	if !ok {
		return nil, errors.New("map not found")
	}

	return m.Optimize(nil)
}

func handleMapMeasurementsRequest(ar *api.Request) (i interface{}, err error) {
	// Get map.
	m, ok := getMapForAPI(ar.URLVars["map"])
	if !ok {
		return nil, errors.New("map not found")
	}

	// Get and sort pins.
	list := m.pinList(true)
	sort.Sort(sortByLowestMeasuredCost(list))

	// Copy data and return.
	measurements := make([]*hub.Measurements, 0, len(list))
	for _, pin := range list {
		measurements = append(measurements, pin.measurements.Copy())
	}
	return measurements, nil
}

func handleMapMeasurementsTableRequest(ar *api.Request) (data []byte, err error) {
	// Get map.
	m, ok := getMapForAPI(ar.URLVars["map"])
	if !ok {
		return nil, errors.New("map not found")
	}
	matcher := m.DefaultOptions().Matcher(TransitHub)

	// Get and sort pins.
	list := m.pinList(true)
	sort.Sort(sortByLowestMeasuredCost(list))

	// Build table and return.
	buf := bytes.NewBuffer(nil)
	tabWriter := tabwriter.NewWriter(buf, 8, 4, 3, ' ', 0)
	fmt.Fprint(tabWriter, "Remote\tCountry\tLatency\tCapacity\tCost\n")
	for _, pin := range list {
		// Only print regarded Hubs.
		if !matcher(pin) {
			continue
		}

		// Add row.
		pin.measurements.Lock()
		defer pin.measurements.Unlock()
		fmt.Fprint(tabWriter, strings.Join([]string{
			pin.Hub.Name(),
			getPinCountry(pin),
			pin.measurements.Latency.String(),
			fmt.Sprintf("%.2fMbit/s", float64(pin.measurements.Capacity)/1000000),
			fmt.Sprintf("%.2fc", pin.measurements.CalculatedCost),
		}, "\t"))

		// Add linebreak.
		fmt.Fprint(tabWriter, "\n")
	}
	tabWriter.Flush()

	return buf.Bytes(), nil
}

func getPinCountry(pin *Pin) string {
	switch {
	case pin.LocationV4 != nil && pin.LocationV4.Country.ISOCode != "":
		return pin.LocationV4.Country.ISOCode
	case pin.LocationV6 != nil && pin.LocationV6.Country.ISOCode != "":
		return pin.LocationV6.Country.ISOCode
	case pin.EntityV4 != nil && pin.EntityV4.Country != "":
		return pin.EntityV4.Country
	case pin.EntityV6 != nil && pin.EntityV6.Country != "":
		return pin.EntityV6.Country
	default:
		return ""
	}
}

func handleMapGraphRequest(w http.ResponseWriter, hr *http.Request) {
	r := api.GetAPIRequest(hr)
	if r == nil {
		http.Error(w, "API request invalid.", http.StatusInternalServerError)
		return
	}

	// Get map.
	m, ok := getMapForAPI(r.URLVars["map"])
	if !ok {
		http.Error(w, "Map not found.", http.StatusNotFound)
		return
	}

	// Check format.
	var format string
	switch r.URLVars["format"] {
	case ".dot":
		format = "dot"
	case ".html":
		format = "html"

		// Check if we are in dev mode.
		if !devMode() {
			http.Error(w, "Graph html formatting (js rendering) is only available in dev mode.", http.StatusPreconditionFailed)
			return
		}
	default:
		http.Error(w, "Unsupported format.", http.StatusBadRequest)
		return
	}

	// Build graph.
	graph := gographviz.NewGraph()
	graph.AddAttr("", "ranksep", "0.2")
	graph.AddAttr("", "nodesep", "0.5")
	graph.AddAttr("", "center", "true")
	graph.AddAttr("", "rankdir", "LR")
	graph.AddAttr("", "ratio", "fill")
	for _, pin := range m.sortedPins(true) {
		graph.AddNode("", pin.Hub.ID, map[string]string{
			"label":     graphNodeLabel(pin),
			"tooltip":   graphNodeTooltip(pin),
			"color":     graphNodeBorderColor(pin),
			"fillcolor": graphNodeColor(pin),
			"shape":     "circle",
			"style":     "filled",
			"fontsize":  "12",
			"penwidth":  "2",
			"margin":    "0",
		})
		for _, lane := range pin.ConnectedTo {
			if graph.IsNode(lane.Pin.Hub.ID) && pin.State != StateNone {
				// Create attributes.
				edgeOptions := map[string]string{
					"tooltip": graphEdgeTooltip(lane),
					"color":   graphEdgeColor(pin, lane.Pin, lane),
					"len":     strconv.Itoa(int(lane.Latency / time.Millisecond)),
				}
				if edgeOptions["color"] == graphColorHomeAndConnected {
					edgeOptions["penwidth"] = "2"
				}
				// Add edge.
				graph.AddEdge(pin.Hub.ID, lane.Pin.Hub.ID, false, edgeOptions)
			}
		}
	}

	var mimeType string
	var responseData []byte
	switch format {
	case "dot":
		mimeType = "text/x-dot"
		responseData = []byte(graph.String())
	case "html":
		mimeType = "text/html"
		responseData = []byte(fmt.Sprintf(
			`<!DOCTYPE html><html><meta charset="utf-8"><body style="margin:0;padding:0;">
<style>#graph svg {height: 99.5vh; width: 99.5vw;}</style>
<div id="graph"></div>
<script src="https://cdn.jsdelivr.net/npm/@hpcc-js/wasm@1.11.0/dist/index.min.js" integrity="sha256-ddqQRurJoGHtZfPh6lth44TYGG5dHRxgHJjnqeOVN2Y=" crossorigin="anonymous"></script>
<script src="https://cdn.jsdelivr.net/npm/d3@7.0.1/dist/d3.min.js" integrity="sha256-rw249VxIkeE54bKM2Cl2L7BIwIeVYNfFOaJ8it1ODvo=" crossorigin="anonymous"></script>
<script src="https://cdn.jsdelivr.net/npm/d3-graphviz@4.0.0/build/d3-graphviz.min.js" integrity="sha256-i+M3EvUd72UcF7LuKZm4eACil5o5qIibtX85JyxD5fQ=" crossorigin="anonymous"></script>
<script>
d3.select("#graph").graphviz(useWorker=false).renderDot(%s%s%s);
</script>
</body></html>`,
			"`", graph.String(), "`",
		))
	}

	// Write response.
	w.Header().Set("Content-Type", mimeType+"; charset=utf-8")
	w.Header().Set("Content-Length", strconv.Itoa(len(responseData)))
	w.WriteHeader(http.StatusOK)
	_, err := w.Write(responseData)
	if err != nil {
		log.Tracer(r.Context()).Warningf("api: failed to write response: %s", err)
	}
}

func graphNodeLabel(pin *Pin) (s string) {
	var comment string
	switch {
	case pin.State == StateNone:
		comment = "dead"
	case pin.State.has(StateIsHomeHub):
		comment = "Home"
	case pin.State.hasAnyOf(StateSummaryDisregard):
		comment = "disregarded"
	case !pin.State.has(StateSummaryRegard):
		comment = "not regarded"
	case pin.State.has(StateTrusted):
		comment = "trusted"
	}
	if comment != "" {
		comment = fmt.Sprintf("\n(%s)", comment)
	}

	if pin.Hub.Status.Load >= 80 {
		comment += fmt.Sprintf("\nHIGH LOAD: %d", pin.Hub.Status.Load)
	}

	return fmt.Sprintf(
		`"%s%s"`,
		strings.ReplaceAll(pin.Hub.Name(), " ", "\n"),
		comment,
	)
}

func graphNodeTooltip(pin *Pin) string {
	// Gather IP info.
	var v4Info, v6Info string
	if pin.Hub.Info.IPv4 != nil {
		if pin.LocationV4 != nil {
			v4Info = fmt.Sprintf("%s (%s)", pin.Hub.Info.IPv4.String(), pin.LocationV4.Country.ISOCode)
		} else {
			v4Info = pin.Hub.Info.IPv4.String()
		}
	}
	if pin.Hub.Info.IPv6 != nil {
		if pin.LocationV6 != nil {
			v6Info = fmt.Sprintf("%s (%s)", pin.Hub.Info.IPv6.String(), pin.LocationV6.Country.ISOCode)
		} else {
			v6Info = pin.Hub.Info.IPv6.String()
		}
	}

	return fmt.Sprintf(
		`"ID: %s
States: %s
IPv4: %s
IPv6: %s
Load: %d
Cost: %.2f"`,
		pin.Hub.ID,
		pin.State,
		v4Info,
		v6Info,
		pin.Hub.Status.Load,
		pin.Cost,
	)
}

func graphEdgeTooltip(lane *Lane) string {
	return fmt.Sprintf(
		`"Latency: %s
Capacity: %.2f Mbit/s
Cost: %.2f"`,
		lane.Latency,
		float64(lane.Capacity)/1000000,
		lane.Cost,
	)
}

// Graphviz colors.
// See https://graphviz.org/doc/info/colors.html
const (
	graphColorWarning          = "orange2"
	graphColorError            = "red2"
	graphColorHomeAndConnected = "steelblue2"
	graphColorDisregard        = "tomato2"
	graphColorNotRegard        = "tan2"
	graphColorTrusted          = "seagreen2"
	graphColorDefaultNode      = "seashell2"
	graphColorDefaultEdge      = "black"
	graphColorNone             = "transparent"
)

func graphNodeColor(pin *Pin) string {
	switch {
	case pin.State == StateNone:
		return graphColorNone
	case pin.Hub.Status.Load >= 95:
		return graphColorError
	case pin.Hub.Status.Load >= 80:
		return graphColorWarning
	case pin.State.has(StateIsHomeHub):
		return graphColorHomeAndConnected
	case pin.State.hasAnyOf(StateSummaryDisregard):
		return graphColorDisregard
	case !pin.State.has(StateSummaryRegard):
		return graphColorNotRegard
	case pin.State.has(StateTrusted):
		return graphColorTrusted
	default:
		return graphColorDefaultNode
	}
}

func graphNodeBorderColor(pin *Pin) string {
	switch {
	case pin.HasActiveTerminal():
		return graphColorHomeAndConnected
	default:
		return graphColorNone
	}
}

func graphEdgeColor(from, to *Pin, lane *Lane) string {
	// Check lane stats.
	if lane.Capacity == 0 || lane.Latency == 0 {
		return graphColorWarning
	}
	// Alert if capacity is under 10Mbit/s or latency is over 100ms.
	if lane.Capacity < 10000000 || lane.Latency > 100*time.Millisecond {
		return graphColorError
	}

	// Check for active edge forward.
	if to.HasActiveTerminal() && len(to.Connection.Route.Path) >= 2 {
		secondLastHopIndex := len(to.Connection.Route.Path) - 2
		if to.Connection.Route.Path[secondLastHopIndex].HubID == from.Hub.ID {
			return graphColorHomeAndConnected
		}
	}
	// Check for active edge backward.
	if from.HasActiveTerminal() && len(from.Connection.Route.Path) >= 2 {
		secondLastHopIndex := len(from.Connection.Route.Path) - 2
		if from.Connection.Route.Path[secondLastHopIndex].HubID == to.Hub.ID {
			return graphColorHomeAndConnected
		}
	}

	// Return default color if edge is not active.
	return graphColorDefaultEdge
}

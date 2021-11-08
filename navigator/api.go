package navigator

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/awalterschulze/gographviz"
	"github.com/safing/portbase/api"
	"github.com/safing/portbase/log"
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
		Description: "Returns a list of pins on the given SPN map.",
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
			"tooltip":   graphTooltip(pin),
			"color":     graphNodeBorderColor(pin),
			"fillcolor": graphNodeColor(pin),
			"shape":     "circle",
			"style":     "filled",
			"fontsize":  "12",
			"penwidth":  "2",
			"margin":    "0",
		})
		for _, lane := range pin.ConnectedTo {
			if graph.IsNode(lane.Pin.Hub.ID) {
				// Create attributes.
				edgeOptions := map[string]string{
					"color": graphEdgeColor(pin, lane.Pin),
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

	return fmt.Sprintf(
		`"%s%s"`,
		strings.ReplaceAll(pin.Hub.Name(), " ", "\n"),
		comment,
	)
}

func graphTooltip(pin *Pin) string {
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
IPv6: %s"`,
		pin.Hub.ID,
		pin.State,
		v4Info,
		v6Info,
	)
}

const (
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

func graphEdgeColor(from, to *Pin) string {
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

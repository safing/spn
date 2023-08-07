package access

// Feature describes a notable part of the program.
type Feature struct {
	Name              string
	ConfigKey         string
	ConfigScope       string
	RequiredFeatureID string
	InPackage         *Package
	Comment           string
}

// Package combines a set of features.
type Package struct {
	Name     string
	HexColor string
	InfoURL  string
}

var (
	infoURL     = "https://safing.io/pricing/"
	packageFree = &Package{
		Name:     "Free",
		HexColor: "#ffffff",
		InfoURL:  infoURL,
	}
	packagePlus = &Package{
		Name:     "Plus",
		HexColor: "#2fcfae",
		InfoURL:  infoURL,
	}
	packagePro = &Package{
		Name:     "Pro",
		HexColor: "#029ad0",
		InfoURL:  infoURL,
	}
	features = []Feature{
		{
			Name:        "Secure DNS",
			ConfigScope: "dns/",
			InPackage:   packageFree,
		},
		{
			Name:        "Privacy Filter",
			ConfigKey:   "filter/enable",
			ConfigScope: "filter/",
			InPackage:   packageFree,
		},
		{
			Name:        "Network History",
			ConfigKey:   "history/enable",
			ConfigScope: "history/",
			InPackage:   packagePlus,
			Comment:     "In Beta",
		},
		{
			Name:      "Bandwidth Visibility",
			InPackage: packagePlus,
			Comment:   "Coming Soon",
		},
		{
			Name:      "Safing Privacy Network",
			InPackage: packagePro,
		},
		{
			Name:      "Priority Support",
			InPackage: packagePlus,
		},
	}
)

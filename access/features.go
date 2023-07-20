package access

// Feature describes a notable part of the program.
type Feature struct {
	Name              string
	ConfigKey         string
	ConfigScope       string
	RequiredFeatureID string // FIXME: can more than one be required?
	InPackage         *Package
	Comment           string
}

// Package combines a set of features.
type Package struct {
	Name     string
	HexColor string
}

var (
	packageFree = &Package{
		Name:     "Free",
		HexColor: "ffffff",
	}
	packagePlus = &Package{
		Name:     "Plus",
		HexColor: "2fcfae",
	}
	packagePro = &Package{
		Name:     "Pro",
		HexColor: "029ad0",
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
			ConfigKey:   "history/enabled", // FIXME: other settings use present tense - change to "history/enable"?
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

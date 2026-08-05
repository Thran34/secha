package report

type Palette struct {
	Name          string
	Surface       string
	TextPrimary   string
	TextSecondary string
	Muted         string
	Grid          string
	Axis          string
	Series        []string
	SeriesWash    string
}

var Light = Palette{
	Name:          "light",
	Surface:       "#fcfcfb",
	TextPrimary:   "#0b0b0b",
	TextSecondary: "#52514e",
	Muted:         "#898781",
	Grid:          "#e1e0d9",
	Axis:          "#c3c2b7",
	Series:        []string{"#2a78d6", "#eb6834", "#1baf7a"},
	SeriesWash:    "#9ec5f4",
}

var Dark = Palette{
	Name:          "dark",
	Surface:       "#1a1a19",
	TextPrimary:   "#ffffff",
	TextSecondary: "#c3c2b7",
	Muted:         "#898781",
	Grid:          "#2c2c2a",
	Axis:          "#383835",
	Series:        []string{"#3987e5", "#d95926", "#199e70"},
	SeriesWash:    "#1c5cab",
}

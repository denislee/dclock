package main

type Location struct {
	Name      string
	Latitude  float64
	Longitude float64
}

// Locations is the preset city list shown in the settings dropdown.
// To add a city: append an entry here.
var Locations = []Location{
	{"Kyiv", 50.4501, 30.5234},
	{"Kharkiv", 49.9935, 36.2304},
	{"Lviv", 49.8397, 24.0297},
	{"Odesa", 46.4825, 30.7233},
	{"Dnipro", 48.4647, 35.0462},
	{"Warsaw", 52.2297, 21.0122},
	{"Berlin", 52.5200, 13.4050},
	{"Vienna", 48.2082, 16.3738},
	{"Amsterdam", 52.3676, 4.9041},
	{"London", 51.5074, -0.1278},
	{"Paris", 48.8566, 2.3522},
	{"Madrid", 40.4168, -3.7038},
	{"Lisbon", 38.7223, -9.1393},
	{"Rome", 41.9028, 12.4964},
	{"Athens", 37.9838, 23.7275},
	{"Istanbul", 41.0082, 28.9784},
	{"Dubai", 25.2048, 55.2708},
	{"New York", 40.7128, -74.0060},
	{"San Francisco", 37.7749, -122.4194},
	{"São Paulo", -23.5505, -46.6333},
	{"Tokyo", 35.6762, 139.6503},
	{"Singapore", 1.3521, 103.8198},
	{"Sydney", -33.8688, 151.2093},
}

// LocationIndex returns the index of the preset matching cfg's name (case-insensitive)
// or -1 if there isn't a match.
func LocationIndex(name string) int {
	for i, loc := range Locations {
		if equalFold(loc.Name, name) {
			return i
		}
	}
	return -1
}

func equalFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if ca >= 'A' && ca <= 'Z' {
			ca += 32
		}
		if cb >= 'A' && cb <= 'Z' {
			cb += 32
		}
		if ca != cb {
			return false
		}
	}
	return true
}

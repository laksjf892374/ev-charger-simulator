package app

import (
	"encoding/json"
	"fmt"

	"cposim/controller/behavior"
	"cposim/controller/charger"
	"cposim/entity"
)

// seed creates the demo world: one charger that never fails (so a first try always works), one
// that is as reliable as a real one (the default for any new charger), and one set up to fail.
// It goes through the controllers like any other caller, so the eMSP hears about the seeded sites
// and chargers the same way it hears about later ones.
func (w *world) seed() error {
	seededSites := []struct {
		chargers []charger.AddChargerInput
		site     entity.Site
	}{
		{
			chargers: []charger.AddChargerInput{
				{Behaviors: []entity.BehaviorSpec{}, MaxPowerKW: 150, PricePerKWH: 0.55},
				{MaxPowerKW: 50},
			},
			site: entity.Site{
				Address:   "1 Broadway",
				City:      "Oakland",
				Country:   "USA",
				Latitude:  37.7955,
				Longitude: -122.2764,
				Name:      "Downtown Fast Charging Hub",
			},
		},
		{
			chargers: []charger.AddChargerInput{
				{
					Behaviors: []entity.BehaviorSpec{{
						Kind:   behavior.KindStartFails,
						Params: json.RawMessage(`{"delay_s": 8}`),
					}},
					MaxPowerKW:  11,
					PricePerKWH: 0.32,
				},
			},
			site: entity.Site{
				Address:   "2100 Shattuck Ave",
				City:      "Berkeley",
				Country:   "USA",
				Latitude:  37.8703,
				Longitude: -122.2680,
				Name:      "Shattuck Parking Garage",
			},
		},
	}

	for _, seeded := range seededSites {
		addedSite, err := w.chargerController.AddSite(seeded.site)
		if err != nil {
			return fmt.Errorf("chargerController.AddSite: %w", err)
		}

		for _, input := range seeded.chargers {
			input.SiteID = addedSite.SiteID

			if _, err := w.chargerController.AddCharger(input); err != nil {
				return fmt.Errorf("chargerController.AddCharger: %w", err)
			}
		}
	}

	return nil
}

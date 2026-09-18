package charger

import (
	"fmt"
	"math"

	"cposim/entity"
)

func (c *controller) AddSite(site entity.Site) (entity.Site, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := validateSite(site); err != nil {
		return entity.Site{}, fmt.Errorf("validateSite: %w", err)
	}

	sites, err := c.siteRepository.List()
	if err != nil {
		return entity.Site{}, fmt.Errorf("siteRepository.List: %w", err)
	}

	if len(sites) >= c.config.MaxSites {
		return entity.Site{}, fmt.Errorf("site limit reached: limit %d", c.config.MaxSites)
	}

	if site.SiteID == "" {
		site.SiteID = c.identifierGateway.NewID(siteIDPrefix)
	} else if _, err := c.siteRepository.Get(site.SiteID); err == nil {
		return entity.Site{}, fmt.Errorf("site %q already exists", site.SiteID)
	}

	site.UpdatedAt = c.clockGateway.Now()

	if err := c.siteRepository.Upsert(site); err != nil {
		return entity.Site{}, fmt.Errorf("siteRepository.Upsert: %w", err)
	}

	if err := c.eventsGateway.PublishSiteEvent(site); err != nil {
		return entity.Site{}, fmt.Errorf("eventsGateway.PublishSiteEvent: %w", err)
	}

	return site, nil
}

func validateSite(site entity.Site) error {
	if site.Name == "" {
		return fmt.Errorf("site name must not be empty")
	}

	if site.SiteID != "" && !validID.MatchString(site.SiteID) {
		return fmt.Errorf("site ID must be 1-36 letters, digits, '_' or '-': site ID %q", site.SiteID)
	}

	for _, text := range []string{site.Address, site.City, site.CountryCode, site.Name} {
		if len(text) > maxTextLength {
			return fmt.Errorf("site text fields must be at most %d characters", maxTextLength)
		}
	}

	if math.Abs(site.Latitude) > 90 || math.Abs(site.Longitude) > 180 {
		return fmt.Errorf("site coordinates are out of range: latitude %v, longitude %v", site.Latitude, site.Longitude)
	}

	return nil
}

func (c *controller) GetSite(siteID string) (entity.Site, error) {
	site, err := c.siteRepository.Get(siteID)
	if err != nil {
		return entity.Site{}, fmt.Errorf("siteRepository.Get: %w", err)
	}

	return site, nil
}

func (c *controller) ListSites() ([]entity.Site, error) {
	sites, err := c.siteRepository.List()
	if err != nil {
		return nil, fmt.Errorf("siteRepository.List: %w", err)
	}

	return sites, nil
}

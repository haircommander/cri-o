package userns

import (
	"math"
	"strings"

	"github.com/containers/storage/pkg/idtools"
	"github.com/cri-o/cri-o/pkg/types"
)

var (
	intMax    int           = calculateIntMax()
	fullIDMap idtools.IDMap = idtools.IDMap{
		ContainerID: 0,
		HostID:      0,
		Size:        intMax,
	}
)

func calculateIntMax() int {
	max := int64(int(^uint(0) >> 1))
	if max > math.MaxUint32 {
		max = math.MaxUint32
	}
	return int(max)
}

type Config struct {
	idMappings *idtools.IDMappings
}

func New() *Config {
	return &Config{}
}

func (c *Config) IDMappings() *idtools.IDMappings {
	return c.idMappings
}

func (c *Config) LoadIDMappings(uidMappings, gidMappings string) error {
	if uidMappings == "" || gidMappings == "" {
		return nil
	}

	parsedUIDsMappings, err := idtools.ParseIDMap(strings.Split(uidMappings, ","), "UID")
	if err != nil {
		return err
	}
	parsedGIDsMappings, err := idtools.ParseIDMap(strings.Split(gidMappings, ","), "GID")
	if err != nil {
		return err
	}

	c.idMappings = idtools.NewIDMappingsFromMaps(parsedUIDsMappings, parsedGIDsMappings)
	return nil
}

func (c *Config) Info() types.IDMappings {
	if c.idMappings == nil {
		return types.IDMappings{
			Uids: []idtools.IDMap{fullIDMap},
			Gids: []idtools.IDMap{fullIDMap},
		}
	}
	return types.IDMappings{
		Uids: c.idMappings.UIDs(),
		Gids: c.idMappings.GIDs(),
	}
}

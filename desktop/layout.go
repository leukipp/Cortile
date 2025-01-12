package desktop

import (
	"github.com/leukipp/cortile/v2/common"
	"github.com/leukipp/cortile/v2/store"
)

type Layout interface {
	Reset()
	Apply()
	AddClient(c *store.Client)
	RemoveClient(c *store.Client)
	MakeMaster(c *store.Client)
	SwapClient(c1 *store.Client, c2 *store.Client)
	ActiveClient() *store.Client
	NextClient() *store.Client
	PreviousClient() *store.Client
	DirectionClient(d common.Direction) *store.Client
	IncreaseMaster()
	DecreaseMaster()
	IncreaseSlave()
	DecreaseSlave()
	IncreaseProportion()
	DecreaseProportion()
	UpdateProportions(c *store.Client, d *store.Directions)
	GetManager() *store.Manager
	GetName() string
}

package state

import (
	"github.com/VictoriaMetrics/fastcache"
	cmap "github.com/orcaman/concurrent-map"
)

var stateObjCacheMap = cmap.New()

var stateObjHits int = 0
var stateObjFastCache = fastcache.New(3 * 1024 * 1024 * 1024)
var stateObjCodeFastCache = fastcache.New(6 * 1024 * 1024 * 1024)

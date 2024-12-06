package growthbook

import (
	"net/http"
	"sync"

	"github.com/growthbook/growthbook-golang/internal/condition"
)

type data struct {
	mu            sync.RWMutex
	features      FeatureMap
	savedGroups   condition.SavedGroups
	apiHost       string
	clientKey     string
	decryptionKey string
	httpClient    *http.Client
}

func newData() *data {
	return &data{}
}

package growthbook

import (
	"net/http"
	"sync"
)

type data struct {
	mu            sync.RWMutex
	features      FeatureMap
	apiHost       string
	clientKey     string
	decryptionKey string
	httpClient    *http.Client
}

func newData() *data {
	return &data{}
}

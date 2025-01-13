package growthbook

import (
	"context"
	"sync"
	"testing"
)

var gbFeature1JSON = `{
    "pro.organizations": {
		"rules": [
            {
                "condition": {
                    "email": {
                        "$regex": "\\+\\d+organizations@example.org$"
                    }
                },
                "force": true
            }
		]
    }
}`

func Test_GrowthBookDeadLock(t *testing.T) {
	ctx := context.Background()
	client, _ := NewClient(ctx, WithJsonFeatures(gbFeature1JSON))

	wg := new(sync.WaitGroup)
	wg.Add(8)
	for i := 0; i < 8; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < 10240; j++ {
				child, _ := client.WithAttributes(Attributes{
					"userID": "some_user_id",
					"email":  "some_email",
				})
				featureRes := child.EvalFeature(ctx, "pro.organizations")
				_ = featureRes
			}
		}()
	}
	wg.Wait()
}

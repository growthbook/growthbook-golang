package growthbook

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
)

func main() {
	context := Context{}
	growthbook := GrowthBook{&context}

	resp, err := http.Get("https://s3.amazonaws.com/myBucket/features.json")
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatal(err)
	}
	features, err := ParseFeatureMap(body)
	if err != nil {
		log.Fatal(err)
	}
	growthbook.SetFeatures(features)

	if growthbook.Feature("my-feature").On {
		// ...
	}

	color := growthbook.Feature("signup-button-color").GetValueWithDefault("blue")
	fmt.Println(color)

	result := growthbook.Run(&Experiment{
		Key:        "my-experiment",
		Variations: []interface{}{"A", "B"},
	})

	fmt.Println(result.Value)

	cov := 0.5
	result2 := growthbook.Run(&Experiment{
		Key: "complex-experiment",
		Variations: []interface{}{
			map[string]string{"color": "blue", "size": "small"},
			map[string]string{"color": "green", "size": "large"},
		},
		Weights:  []float64{0.8, 0.2},
		Coverage: &cov,
		// Condition: { beta: true },
	})
	fmt.Println(result2.Value.(map[string]string)["color"],
		result2.Value.(map[string]string)["size"])
}

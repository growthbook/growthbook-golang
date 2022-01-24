package growthbook

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
)

func main() {
	// Set up development logger: logs all messages from GrowthBook SDK
	// and exits on errors.
	SetLogger(&DevLogger{})

	// Download JSON feature file and read file body.
	resp, err := http.Get("https://s3.amazonaws.com/myBucket/features.json")
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatal(err)
	}

	// Parse feature map from JSON.
	features := ParseFeatureMap(body)

	// Create context and main GrowthBook object.
	context := NewContext().WithFeatures(features)
	growthbook := New(context)

	// Perform feature test.
	if growthbook.Feature("my-feature").On {
		// ...
	}

	color := growthbook.Feature("signup-button-color").GetValueWithDefault("blue")
	fmt.Println(color)

	experiment :=
		NewExperiment("my-experiment").
			WithVariations("A", "B")

	result := growthbook.Run(experiment)

	fmt.Println(result.Value)

	experiment2 :=
		NewExperiment("complex-experiment").
			WithVariations(
				map[string]string{"color": "blue", "size": "small"},
				map[string]string{"color": "green", "size": "large"},
			).
			WithWeights(0.8, 0.2).
			WithCoverage(0.5)

	result2 := growthbook.Run(experiment2)
	fmt.Println(result2.Value.(map[string]string)["color"],
		result2.Value.(map[string]string)["size"])
}

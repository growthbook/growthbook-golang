/*
Package growthbook provides a Go SDK for the GrowthBook A/B testing
and feature flagging service.

   CONTEXT SETUP
   GrowthBook SETUP


*/
package growthbook

/*
Error handling:

The GrowthBook public API does not return errors under any normal
circumstances. The intention is for developers to be able to use the
SDK in both development and production smoothly. To this end, error
reporting is provided by a configurable logging interface.

For development use, the DevLogger type provides a suitable
implementation of the logging interface: it prints all logged messages
to standard output, and exits on errors.

For production use, a logger that directs log messages to a suitable
centralised logging facility and ignores all errors would be suitable.
The logger can of course also signal error and warning conditions to
other parts of the program in which it is used.

To be specific about this:

 - None of the functions that create or update Context or Experiment
   values return errors.

 - The main GrowthBook.Feature and GrowthBook.Run functions never
   return errors.

 - None of the functions that create values from JSON data return
   errors.

For most common use cases, this means that the GrowthBook SDK can be
used transparently, without needing to care about error handling. Your
server code will never crash because of problems in the GrowthBook
SDK. The only effect of error conditions in the inputs to the SDK may
be that feature values and results of experiments are not what you
expect.

*/

/*
JSON data representations:

For interoperability of the GrowthBook Go SDK with versions of the SDK
in other languages, the core "input" values of the SDK (in particular,
Context and Experiment values and maps of feature definitions) can be
created by parsing JSON data. A common use case is to download feature
definitions from a central location as JSON, to parse them into a
feature map that can be applied to a GrowthBook Context, then using
this context to create a GrowthBook value that can be used for feature
tests.

A contrived example of how this might work is:

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
	features, err := ParseFeatureMap(body)
	if err != nil {
		log.Fatal(err)
	}

	// Create context and main GrowthBook object.
	context := NewContext().WithFeatures(features)
	growthbook := New(context)

	// Perform feature test.
	if growthbook.Feature("my-feature").On {
		// ...
	}

The functions that implement this JSON processing functionality have
names like ParseContext, BuildContext, and so on. Each Parse...
function process raw JSON data (as a []byte) value, while the Build...
functions process JSON objects unmarshalled to Go values of type
map[string]interface{}. This provides flexibility in ingestion of JSON
data.

*/

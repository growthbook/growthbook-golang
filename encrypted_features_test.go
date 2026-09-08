package growthbook

import (
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	testDecryptionKey  = "Zvwv/+uhpFDznZ6SX28Yjg=="
	testEncryptedFeats = "m5ylFM6ndyOJA2OPadubkw==.Uu7ViqgKEt/dWvCyhI46q088PkAEJbnXKf3KPZjf9IEQQ+A8fojNoxw4wIbPX3aj"
)

func TestEncryptedFeaturesWorkWithTheKeyOptionListedSecond(t *testing.T) {
	client, err := NewClient(ctx,
		WithEncryptedJsonFeatures(testEncryptedFeats),
		WithDecryptionKey(testDecryptionKey),
	)

	require.NoError(t, err, "option order must not decide whether a supplied decryption key is used")
	require.NotNil(t, client)
}

func TestEncryptedFeaturesWorkWithTheKeyOptionListedFirst(t *testing.T) {
	client, err := NewClient(ctx,
		WithDecryptionKey(testDecryptionKey),
		WithEncryptedJsonFeatures(testEncryptedFeats),
	)

	require.NoError(t, err)
	require.NotNil(t, client)
}

func TestBothOptionOrdersProduceTheSameFeatures(t *testing.T) {
	keyFirst, err := NewClient(ctx,
		WithDecryptionKey(testDecryptionKey),
		WithEncryptedJsonFeatures(testEncryptedFeats),
	)
	require.NoError(t, err)

	featuresFirst, err := NewClient(ctx,
		WithEncryptedJsonFeatures(testEncryptedFeats),
		WithDecryptionKey(testDecryptionKey),
	)
	require.NoError(t, err)

	require.Equal(t, keyFirst.data.getFeatures(), featuresFirst.data.getFeatures())
}

func TestEncryptedFeaturesWithoutAnyKeyStillFails(t *testing.T) {
	_, err := NewClient(ctx, WithEncryptedJsonFeatures(testEncryptedFeats))

	require.ErrorIs(t, err, ErrNoDecryptionKey)
}

func TestEncryptedFeaturesWithTheWrongKeyFails(t *testing.T) {
	_, err := NewClient(ctx,
		WithEncryptedJsonFeatures(testEncryptedFeats),
		WithDecryptionKey("aaaaaaaaaaaaaaaaaaaaaa=="),
	)

	require.Error(t, err, "a bad key has to surface rather than leaving the client with no features")
}

func TestEncryptedFeaturesWithMalformedCiphertextFails(t *testing.T) {
	_, err := NewClient(ctx,
		WithEncryptedJsonFeatures("not-a-payload"),
		WithDecryptionKey(testDecryptionKey),
	)

	require.Error(t, err)
}

func TestSetEncryptedJSONFeaturesStillRequiresAKeyWhenCalledDirectly(t *testing.T) {
	client, err := NewClient(ctx)
	require.NoError(t, err)

	require.ErrorIs(t, client.SetEncryptedJSONFeatures(testEncryptedFeats), ErrNoDecryptionKey)
}

func TestAClientWithNoEncryptedFeaturesIsUnaffected(t *testing.T) {
	client, err := NewClient(ctx,
		WithJsonFeatures(`{"feature": {"defaultValue": 1}}`),
		WithDecryptionKey(testDecryptionKey),
	)
	require.NoError(t, err)

	require.EqualValues(t, 1, client.EvalFeature(ctx, "feature").Value)
}

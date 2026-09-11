package growthbook

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
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

func TestAPlainFeatureOptionListedAfterEncryptedOneWins(t *testing.T) {
	// Deferring decryption must not move it past unrelated feature options - the last feature option
	// listed is still the one that decides.
	client, err := NewClient(ctx,
		WithDecryptionKey(testDecryptionKey),
		WithEncryptedJsonFeatures(testEncryptedFeats),
		WithJsonFeatures(`{"fallback":{"defaultValue":"plain"}}`),
	)
	require.NoError(t, err)

	require.Equal(t, "plain", client.EvalFeature(ctx, "fallback").Value,
		"the plain option came last, so it has to win")
	require.Nil(t, client.EvalFeature(ctx, "feature").Value,
		"and the encrypted payload it superseded must not come back after the option loop")
}

func TestAnEncryptedFeatureOptionListedAfterAPlainOneWins(t *testing.T) {
	client, err := NewClient(ctx,
		WithJsonFeatures(`{"fallback":{"defaultValue":"plain"}}`),
		WithEncryptedJsonFeatures(testEncryptedFeats),
		WithDecryptionKey(testDecryptionKey),
	)
	require.NoError(t, err)

	require.Equal(t, true, client.EvalFeature(ctx, "feature").Value, "the encrypted option came last")
	require.Nil(t, client.EvalFeature(ctx, "fallback").Value)
}

func TestAnInvalidEncryptedPayloadFailsEvenWhenAValidOneFollowsIt(t *testing.T) {
	// Collapsing the deferred payloads into one would let a valid payload hide an invalid one instead
	// of failing construction.
	client, err := NewClient(ctx,
		WithEncryptedJsonFeatures("not-a-valid-payload"),
		WithEncryptedJsonFeatures(testEncryptedFeats),
		WithDecryptionKey(testDecryptionKey),
	)

	require.Error(t, err, "every deferred payload is decrypted, so the invalid one still fails construction")
	require.Nil(t, client)
}

func TestTheLastOfSeveralValidEncryptedPayloadsWins(t *testing.T) {
	other, err := encryptFeaturesForTest(`{"other":{"defaultValue":"second"}}`)
	require.NoError(t, err)

	client, err := NewClient(ctx,
		WithDecryptionKey(testDecryptionKey),
		WithEncryptedJsonFeatures(testEncryptedFeats),
		WithEncryptedJsonFeatures(other),
	)
	require.NoError(t, err)

	require.Equal(t, "second", client.EvalFeature(ctx, "other").Value)
	require.Nil(t, client.EvalFeature(ctx, "feature").Value)
}

// encryptFeaturesForTest produces a payload the SDK's own decrypt accepts, so the ordering of two
// valid encrypted options can be tested. The repository ships only one valid ciphertext for this key.
func encryptFeaturesForTest(featuresJson string) (string, error) {
	key, err := base64.StdEncoding.DecodeString(testDecryptionKey)
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	iv := make([]byte, block.BlockSize())
	if _, err := rand.Read(iv); err != nil {
		return "", err
	}

	padding := block.BlockSize() - len(featuresJson)%block.BlockSize()
	padded := append([]byte(featuresJson), bytes.Repeat([]byte{byte(padding)}, padding)...)

	cipherText := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(cipherText, padded)

	return base64.StdEncoding.EncodeToString(iv) + "." + base64.StdEncoding.EncodeToString(cipherText), nil
}

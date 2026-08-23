package auth

// CredentialsVersion is the on-disk credentials.json schema version.
const CredentialsVersion = 2

// LegacyCredentialsVersion is the pre-device-code schema.
const LegacyCredentialsVersion = 1

// StoredCredentialKind discriminates credentials.json payloads.
type StoredCredentialKind string

const (
	KindAPIKey     StoredCredentialKind = "api_key"
	KindDeviceCode StoredCredentialKind = "device_code"
	KindSageMaker  StoredCredentialKind = "sagemaker"
)

// StoredAPIKeyCredentials is a bearer token stored locally.
type StoredAPIKeyCredentials struct {
	Version int                  `json:"version"`
	Kind    StoredCredentialKind `json:"kind"`
	APIKey  string               `json:"api_key"`
}

// StoredDeviceCodeCredentials is OAuth device-code tokens.
type StoredDeviceCodeCredentials struct {
	Version      int                  `json:"version"`
	Kind         StoredCredentialKind `json:"kind"`
	AccessToken  string               `json:"access_token"`
	RefreshToken string               `json:"refresh_token,omitempty"`
	ExpiresAt    string               `json:"expires_at,omitempty"`
	TokenType    string               `json:"token_type,omitempty"`
	Scope        string               `json:"scope,omitempty"`
}

// StoredSageMakerCredentials is a self-hosted SageMaker endpoint config.
type StoredSageMakerCredentials struct {
	Version     int                  `json:"version"`
	Kind        StoredCredentialKind `json:"kind"`
	EndpointARN string               `json:"endpoint_arn"`
	Profile     string               `json:"profile,omitempty"`
}

// StoredCredentials is the union of stored credential kinds.
type StoredCredentials struct {
	Version int                  `json:"version"`
	Kind    StoredCredentialKind `json:"kind"`

	// api_key
	APIKey string `json:"api_key,omitempty"`

	// device_code
	AccessToken  string `json:"access_token,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
	ExpiresAt    string `json:"expires_at,omitempty"`
	TokenType    string `json:"token_type,omitempty"`
	Scope        string `json:"scope,omitempty"`

	// sagemaker
	EndpointARN string `json:"endpoint_arn,omitempty"`
	Profile     string `json:"profile,omitempty"`
}

// LegacyStoredCredentials is the pre-v2 credentials.json shape.
type LegacyStoredCredentials struct {
	Version      int    `json:"version"`
	TakaraAPIKey string `json:"takara_api_key,omitempty"`
	SageMaker    *struct {
		EndpointARN string `json:"endpoint_arn"`
		Profile     string `json:"profile,omitempty"`
	} `json:"sagemaker,omitempty"`
}

// SaveAPIKeyInput saves an API key.
type SaveAPIKeyInput struct {
	APIKey string
}

// SaveDeviceCodeInput saves device-code tokens.
type SaveDeviceCodeInput struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    string
	TokenType    string
	Scope        string
}

// SaveSageMakerInput saves SageMaker endpoint config.
type SaveSageMakerInput struct {
	EndpointARN string
	Profile     string
}

// AuthenticatedCredentials is the result of Takara auth (not SageMaker).
type AuthenticatedCredentials struct {
	Kind         StoredCredentialKind
	APIKey       string
	AccessToken  string
	RefreshToken string
	ExpiresAt    string
	TokenType    string
	Scope        string
}

// CredentialAccessToken returns the bearer token for api_key / device_code kinds.
func CredentialAccessToken(c StoredCredentials) string {
	if c.Kind == KindAPIKey {
		return c.APIKey
	}
	return c.AccessToken
}

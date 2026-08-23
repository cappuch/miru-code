package embed

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sagemakerruntime"
)

var endpointARNPattern = regexp.MustCompile(`^arn:aws[a-z0-9-]*:sagemaker:([a-z0-9-]+):(\d{12}):endpoint/(.+)$`)

// SageMakerConfig mirrors sagemaker.ts.
type SageMakerConfig struct {
	EndpointName        string
	Region              string
	Normalize           bool
	Truncate            bool
	TruncationDirection string
	PromptName          string
}

// ParseSageMakerEndpointARN parses a SageMaker endpoint ARN.
func ParseSageMakerEndpointARN(arn string) (region, accountID, endpointName string, err error) {
	m := endpointARNPattern.FindStringSubmatch(strings.TrimSpace(arn))
	if m == nil {
		return "", "", "", fmt.Errorf(
			`Invalid SageMaker endpoint ARN: %q. Expected format arn:aws:sagemaker:<region>:<account-id>:endpoint/<endpoint-name>.`,
			arn,
		)
	}
	return m[1], m[2], m[3], nil
}

func sageMakerEnvBool(name string, fallback bool) bool {
	raw := strings.ToLower(strings.TrimSpace(os.Getenv(name)))
	if raw == "" {
		return fallback
	}
	return raw != "false" && raw != "0" && raw != "no"
}

// ResolveSageMakerConfig reads MIRU_SAGEMAKER_* env vars.
func ResolveSageMakerConfig() (*SageMakerConfig, error) {
	arn := strings.TrimSpace(os.Getenv("MIRU_SAGEMAKER_ENDPOINT_ARN"))
	explicitName := strings.TrimSpace(os.Getenv("MIRU_SAGEMAKER_ENDPOINT_NAME"))
	if explicitName == "" {
		explicitName = strings.TrimSpace(os.Getenv("MIRU_SAGEMAKER_ENDPOINT"))
	}

	var endpointName, region string
	if arn != "" {
		r, _, name, err := ParseSageMakerEndpointARN(arn)
		if err != nil {
			return nil, err
		}
		endpointName, region = name, r
	} else if explicitName != "" {
		endpointName = explicitName
		region = firstNonEmpty(
			os.Getenv("MIRU_SAGEMAKER_REGION"),
			os.Getenv("AWS_REGION"),
			os.Getenv("AWS_DEFAULT_REGION"),
		)
		if region == "" {
			return nil, fmt.Errorf(
				"MIRU_SAGEMAKER_ENDPOINT_NAME is set but no AWS region was found. Set MIRU_SAGEMAKER_REGION, AWS_REGION, or AWS_DEFAULT_REGION.",
			)
		}
	} else {
		return nil, nil
	}

	trunc := "Right"
	if strings.TrimSpace(os.Getenv("MIRU_SAGEMAKER_TRUNCATION_DIRECTION")) == "Left" {
		trunc = "Left"
	}
	return &SageMakerConfig{
		EndpointName:        endpointName,
		Region:              region,
		Normalize:           sageMakerEnvBool("MIRU_SAGEMAKER_NORMALIZE", true),
		Truncate:            sageMakerEnvBool("MIRU_SAGEMAKER_TRUNCATE", true),
		TruncationDirection: trunc,
		PromptName:          strings.TrimSpace(os.Getenv("MIRU_SAGEMAKER_PROMPT_NAME")),
	}, nil
}

// IsSageMakerConfigured reports whether SageMaker env is set.
func IsSageMakerConfigured() bool {
	cfg, err := ResolveSageMakerConfig()
	return err == nil && cfg != nil
}

// SageMakerAuthErrorMessage is shown for credential failures.
const SageMakerAuthErrorMessage = "Not authorized to invoke this SageMaker endpoint. Check your AWS credentials " +
	"(AWS_ACCESS_KEY_ID/AWS_SECRET_ACCESS_KEY/AWS_SESSION_TOKEN or AWS_PROFILE) and that the " +
	"identity has sagemaker:InvokeEndpoint permission on the endpoint."

// SageMakerNotFoundErrorMessage is shown when the endpoint name no longer exists.
const SageMakerNotFoundErrorMessage =
	"SageMaker endpoint not found — it may have been deleted, renamed, or deployed in a " +
		"different region. Confirm the endpoint still exists, then run `miru setup --sagemaker` " +
		"again to re-authenticate and re-point Miru at the correct endpoint."

// SageMakerUnreachableErrorMessage is shown for DNS/connection-level failures.
const SageMakerUnreachableErrorMessage =
	"Could not reach the SageMaker endpoint (network error). Check your network/VPN/VPC " +
		"access and that the configured region is correct, then run `miru setup --sagemaker` " +
		"again to re-authenticate and re-validate the connection."

var sageMakerNetworkErrorCodes = map[string]struct{}{
	"ENOTFOUND":    {},
	"ECONNREFUSED": {},
	"ECONNRESET":   {},
	"ETIMEDOUT":    {},
	"EAI_AGAIN":    {},
}

// SageMakerInvokeError is an InvokeEndpoint failure.
type SageMakerInvokeError struct {
	Status  int
	Message string
}

func (e *SageMakerInvokeError) Error() string { return e.Message }

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}

type sageMakerRuntimeClient struct {
	cfg    SageMakerConfig
	client *sagemakerruntime.Client
}

func (c *sageMakerRuntimeClient) ensure(ctx context.Context) error {
	if c.client != nil {
		return nil
	}
	opts := []func(*config.LoadOptions) error{config.WithRegion(c.cfg.Region)}
	if profile := strings.TrimSpace(os.Getenv("AWS_PROFILE")); profile != "" {
		opts = append(opts, config.WithSharedConfigProfile(profile))
	}
	awsCfg, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return &SageMakerInvokeError{Status: 403, Message: SageMakerAuthErrorMessage}
	}
	c.client = sagemakerruntime.NewFromConfig(awsCfg)
	return nil
}

func buildSageMakerBody(cfg SageMakerConfig, input []string, dimensions *int) map[string]any {
	body := map[string]any{
		"inputs":               input,
		"normalize":            cfg.Normalize,
		"truncate":             cfg.Truncate,
		"truncation_direction": cfg.TruncationDirection,
	}
	if dimensions != nil {
		body["dimensions"] = *dimensions
	}
	if cfg.PromptName != "" {
		body["prompt_name"] = cfg.PromptName
	}
	return body
}

func extractSageMakerEmbeddings(parsed any) ([]json.RawMessage, error) {
	switch v := parsed.(type) {
	case []any:
		out := make([]json.RawMessage, len(v))
		for i, item := range v {
			b, err := json.Marshal(item)
			if err != nil {
				return nil, err
			}
			out[i] = b
		}
		return out, nil
	case map[string]any:
		if emb, ok := v["embeddings"].([]any); ok {
			out := make([]json.RawMessage, len(emb))
			for i, item := range emb {
				b, err := json.Marshal(item)
				if err != nil {
					return nil, err
				}
				out[i] = b
			}
			return out, nil
		}
		if data, ok := v["data"].([]any); ok {
			out := make([]json.RawMessage, len(data))
			for i, item := range data {
				m, ok := item.(map[string]any)
				if !ok {
					return nil, fmt.Errorf("SageMaker endpoint returned an unrecognized embedding payload shape")
				}
				b, err := json.Marshal(m["embedding"])
				if err != nil {
					return nil, err
				}
				out[i] = b
			}
			return out, nil
		}
	}
	return nil, fmt.Errorf("SageMaker endpoint returned an unrecognized embedding payload shape")
}

func toSageMakerInvokeError(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	for _, n := range []string{
		"AccessDeniedException", "UnrecognizedClientException",
		"ExpiredTokenException", "CredentialsProviderError",
	} {
		if strings.Contains(msg, n) {
			return &SageMakerInvokeError{Status: 403, Message: SageMakerAuthErrorMessage}
		}
	}

	// SageMaker returns a generic ValidationError for both "endpoint doesn't exist" and
	// malformed-request cases — only the message distinguishes them.
	if strings.Contains(msg, "ValidationError") && strings.Contains(strings.ToLower(msg), "not found") {
		return &SageMakerInvokeError{Status: 404, Message: SageMakerNotFoundErrorMessage}
	}
	if strings.Contains(strings.ToLower(msg), "endpoint") && strings.Contains(strings.ToLower(msg), "not found") {
		return &SageMakerInvokeError{Status: 404, Message: SageMakerNotFoundErrorMessage}
	}

	lower := strings.ToLower(msg)
	for code := range sageMakerNetworkErrorCodes {
		if strings.Contains(msg, code) || strings.Contains(lower, strings.ToLower(code)) {
			return &SageMakerInvokeError{Status: 503, Message: SageMakerUnreachableErrorMessage}
		}
	}
	// Go net errors often surface as "dial tcp … connection refused" / "i/o timeout".
	if strings.Contains(lower, "connection refused") ||
		strings.Contains(lower, "i/o timeout") ||
		strings.Contains(lower, "no such host") ||
		strings.Contains(lower, "network is unreachable") {
		return &SageMakerInvokeError{Status: 503, Message: SageMakerUnreachableErrorMessage}
	}

	if strings.Contains(msg, "424") {
		return &SageMakerInvokeError{
			Status:  424,
			Message: fmt.Sprintf("SageMaker endpoint returned 424 — the deployed model is not an embedding model (%s).", msg),
		}
	}
	status := 500
	if i := strings.Index(msg, "StatusCode: "); i >= 0 {
		fields := strings.Fields(msg[i+12:])
		if len(fields) > 0 {
			if n, e := strconv.Atoi(fields[0]); e == nil {
				status = n
			}
		}
	}
	return &SageMakerInvokeError{Status: status, Message: fmt.Sprintf("SageMaker endpoint error %d: %s", status, msg)}
}

type sageMakerEmbeddingClient struct {
	rc *sageMakerRuntimeClient
}

func (c *sageMakerEmbeddingClient) CreateEmbeddings(input []string, _model string, dimensions *int) (*EmbeddingResponse, error) {
	ctx := context.Background()
	if err := c.rc.ensure(ctx); err != nil {
		return nil, err
	}
	raw, err := json.Marshal(buildSageMakerBody(c.rc.cfg, input, dimensions))
	if err != nil {
		return nil, err
	}
	out, err := c.rc.client.InvokeEndpoint(ctx, &sagemakerruntime.InvokeEndpointInput{
		EndpointName: aws.String(c.rc.cfg.EndpointName),
		ContentType:  aws.String("application/json"),
		Accept:       aws.String("application/json"),
		Body:         raw,
	})
	if err != nil {
		return nil, toSageMakerInvokeError(err)
	}
	if len(out.Body) == 0 {
		return nil, fmt.Errorf("SageMaker endpoint returned an empty response body")
	}
	var parsed any
	if err := json.Unmarshal(out.Body, &parsed); err != nil {
		return nil, fmt.Errorf("SageMaker endpoint returned invalid JSON")
	}
	embeddings, err := extractSageMakerEmbeddings(parsed)
	if err != nil {
		return nil, err
	}
	resp := &EmbeddingResponse{Data: make([]EmbeddingResponseItem, len(embeddings))}
	for i, emb := range embeddings {
		resp.Data[i] = EmbeddingResponseItem{Index: i, Embedding: emb}
	}
	return resp, nil
}

// NewSageMakerClient builds an EmbeddingClient for InvokeEndpoint.
func NewSageMakerClient(cfg SageMakerConfig) EmbeddingClient {
	return &sageMakerEmbeddingClient{rc: &sageMakerRuntimeClient{cfg: cfg}}
}

// ValidateSageMakerConnection does a real InvokeEndpoint round trip.
func ValidateSageMakerConnection(cfg SageMakerConfig) (valid bool, status int, message string) {
	client := NewSageMakerClient(cfg)
	resp, err := client.CreateEmbeddings([]string{"miru setup validation"}, "sagemaker-setup", nil)
	if err != nil {
		st := 0
		if ie, ok := err.(*SageMakerInvokeError); ok {
			st = ie.Status
		}
		return false, st, err.Error()
	}
	if len(resp.Data) == 0 || len(resp.Data[0].Embedding) == 0 {
		return false, 0, "SageMaker endpoint returned an empty response."
	}
	return true, 0, "SageMaker endpoint responded successfully."
}

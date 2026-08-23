package embed

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/takara-ai/miru-code/internal/autherr"
	"github.com/takara-ai/miru-code/internal/concurrency"
	"github.com/takara-ai/miru-code/internal/env"
)

const (
	DefaultEmbeddingModel   = "ds1-miru-int8"
	DefaultEmbeddingBaseURL = "https://infer.takara.ai/v1"
	defaultBatchSize        = 360
	defaultMaxEmbedChars    = 1300
	windowOverlapChars      = 120
	maxTransientEmbedRetries = 4
)

var modelDefaultDimensions = map[string]int{
	"ds1-potion-code-16m": 256,
	"ds1-miru-int8":       256,
}

// EmbeddingBackend is the mock-friendly embedding interface.
type EmbeddingBackend interface {
	Model() string
	Dimensions() int
	EmbedDocuments(texts []string) ([][]float32, error)
	EmbedQuery(text string) ([]float32, error)
	// EmbedInputs embeds already-windowed strings (optional for mocks).
	EmbedInputs(texts []string) ([][]float32, error)
}

// EmbeddingClient issues HTTP embedding requests (mockable).
type EmbeddingClient interface {
	CreateEmbeddings(input []string, model string, dimensions *int) (*EmbeddingResponse, error)
}

// EmbeddingResponse matches the OpenAI-compatible payload.
type EmbeddingResponse struct {
	Data []EmbeddingResponseItem `json:"data"`
}

// EmbeddingResponseItem is one vector in a response.
type EmbeddingResponseItem struct {
	Index     int             `json:"index"`
	Embedding json.RawMessage `json:"embedding"`
}

// Int8Embedding is a quantized payload.
type Int8Embedding struct {
	Dtype     string  `json:"dtype"`
	Values    []int   `json:"values"`
	Scale     float64 `json:"scale"`
	ZeroPoint float64 `json:"zero_point"`
}

// EmbeddingTransportStats tracks request metrics.
type EmbeddingTransportStats struct {
	Requests        int
	Retries         int
	PayloadTooLarge int
	Errors          int
	InputItems      int
	InputChars      int
	TotalRttMs      float64
	MaxRttMs        float64
}

// EmbeddingWindowJob ties a window string to its source document.
type EmbeddingWindowJob struct {
	DocIndex int
	Text     string
}

// ResolveTakaraEmbeddingModel returns the Takara model name.
func ResolveTakaraEmbeddingModel() string {
	return env.EnvFirstString(
		[]string{"MIRU_OPENAI_EMBEDDING_MODEL", "OPENAI_EMBEDDING_MODEL"},
		DefaultEmbeddingModel,
	)
}

// ResolveEmbeddingModel returns SageMaker id when configured, else Takara model.
func ResolveEmbeddingModel() string {
	if ep := strings.TrimSpace(os.Getenv("MIRU_SAGEMAKER_ENDPOINT")); ep != "" {
		return SageMakerModelID(ep)
	}
	if arn := strings.TrimSpace(os.Getenv("MIRU_SAGEMAKER_ENDPOINT_ARN")); arn != "" {
		return SageMakerModelID(arn)
	}
	return ResolveTakaraEmbeddingModel()
}

// SageMakerModelID builds the sagemaker: model id stub.
func SageMakerModelID(endpointName string) string {
	return "sagemaker:" + endpointName
}

// ResolveMaxEmbedChars returns the window size (default 1300).
func ResolveMaxEmbedChars() int {
	if v := env.EnvOptionalInt([]string{"MIRU_MAX_EMBED_CHARS"}, 256); v != nil {
		return *v
	}
	return defaultMaxEmbedChars
}

// ResolveEmbeddingDimensions returns known dims or env override.
func ResolveEmbeddingDimensions(model string) *int {
	if v := env.EnvOptionalInt([]string{"MIRU_EMBEDDING_DIMENSIONS", "OPENAI_EMBEDDING_DIMENSIONS"}, 1); v != nil {
		return v
	}
	if model == "" {
		model = ResolveEmbeddingModel()
	}
	if d, ok := modelDefaultDimensions[model]; ok {
		out := d
		return &out
	}
	return nil
}

// ResolveEmbeddingBatchSize returns API batch size.
func ResolveEmbeddingBatchSize() int {
	if v := env.EnvOptionalInt([]string{"MIRU_EMBEDDING_BATCH_SIZE", "OPENAI_EMBEDDING_BATCH_SIZE"}, 1); v != nil {
		return *v
	}
	return defaultBatchSize
}

// ResolveEmbeddingBaseURL returns the OpenAI-compatible base URL.
func ResolveEmbeddingBaseURL() string {
	return strings.TrimRight(env.EnvFirstString(
		[]string{"MIRU_OPENAI_BASE_URL", "OPENAI_BASE_URL"},
		DefaultEmbeddingBaseURL,
	), "/")
}

// SplitIntoWindows splits text into overlapping windows.
func SplitIntoWindows(text string, maxChars int) []string {
	if len(text) <= maxChars {
		return []string{text}
	}
	out := make([]string, 0)
	step := maxChars - windowOverlapChars
	if step < 64 {
		step = 64
	}
	for start := 0; start < len(text); start += step {
		end := start + maxChars
		if end > len(text) {
			end = len(text)
		}
		part := text[start:end]
		if len(part) > 0 {
			out = append(out, part)
		}
		if end >= len(text) {
			break
		}
	}
	return out
}

// SanitizeEmbeddingInput cleans input for JSON embedding APIs.
func SanitizeEmbeddingInput(text string) string {
	out := stripLoneSurrogates(text)
	mode := os.Getenv("MIRU_EMBED_ESCAPE_MODE")
	if mode == "" {
		mode = "preserve"
	}
	switch mode {
	case "quad":
		return strings.ReplaceAll(out, `\`, `\\\\`)
	case "strip":
		return strings.ReplaceAll(out, `\`, `/`)
	default:
		return out
	}
}

func stripLoneSurrogates(text string) string {
	runes := []rune(text)
	out := make([]rune, 0, len(runes))
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if r >= 0xD800 && r <= 0xDBFF {
			if i+1 < len(runes) && runes[i+1] >= 0xDC00 && runes[i+1] <= 0xDFFF {
				out = append(out, r, runes[i+1])
				i++
				continue
			}
			out = append(out, '\uFFFD')
			continue
		}
		if r >= 0xDC00 && r <= 0xDFFF {
			out = append(out, '\uFFFD')
			continue
		}
		out = append(out, r)
	}
	return string(out)
}

// AppendEmbeddingWindowJobs appends sanitized windows for one document.
func AppendEmbeddingWindowJobs(jobs *[]EmbeddingWindowJob, docIndex int, text string, maxChars int) {
	for _, w := range SplitIntoWindows(text, maxChars) {
		*jobs = append(*jobs, EmbeddingWindowJob{DocIndex: docIndex, Text: SanitizeEmbeddingInput(w)})
	}
}

// BuildEmbeddingWindowJobs prepares jobs and empty buckets.
func BuildEmbeddingWindowJobs(texts []string, maxChars int) (jobs []EmbeddingWindowJob, buckets [][][]float32) {
	buckets = make([][][]float32, len(texts))
	for i := range buckets {
		buckets[i] = make([][]float32, 0)
	}
	for docIndex, text := range texts {
		AppendEmbeddingWindowJobs(&jobs, docIndex, text, maxChars)
	}
	return jobs, buckets
}

// BatchEmbeddingWindowJobs splits jobs into batches.
func BatchEmbeddingWindowJobs(jobs []EmbeddingWindowJob, batchSize int) [][]EmbeddingWindowJob {
	if batchSize < 1 {
		batchSize = 1
	}
	batches := make([][]EmbeddingWindowJob, 0)
	for i := 0; i < len(jobs); i += batchSize {
		end := i + batchSize
		if end > len(jobs) {
			end = len(jobs)
		}
		batches = append(batches, jobs[i:end])
	}
	return batches
}

// AssignEmbeddingWindowVectors routes window vectors into document buckets.
func AssignEmbeddingWindowVectors(jobs []EmbeddingWindowJob, vectors [][]float32, buckets [][][]float32) {
	for i, job := range jobs {
		if i >= len(vectors) {
			break
		}
		if job.DocIndex >= 0 && job.DocIndex < len(buckets) {
			buckets[job.DocIndex] = append(buckets[job.DocIndex], vectors[i])
		}
	}
}

func normalize(vec []float32) []float32 {
	var norm float64
	for _, v := range vec {
		norm += float64(v) * float64(v)
	}
	norm = math.Sqrt(norm)
	if norm == 0 {
		return vec
	}
	out := make([]float32, len(vec))
	for i, v := range vec {
		out[i] = float32(float64(v) / norm)
	}
	return out
}

// PoolWindowVectors mean-pools then L2-normalizes.
func PoolWindowVectors(vectors [][]float32) ([]float32, error) {
	if len(vectors) == 0 {
		return nil, fmt.Errorf("Embedding API returned no vectors")
	}
	first := vectors[0]
	if len(vectors) == 1 {
		return normalize(first), nil
	}
	pooled := make([]float32, len(first))
	for _, vec := range vectors {
		for i := 0; i < len(pooled) && i < len(vec); i++ {
			pooled[i] += vec[i]
		}
	}
	n := float32(len(vectors))
	for i := range pooled {
		pooled[i] /= n
	}
	return normalize(pooled), nil
}

// PoolEmbeddingWindowBuckets pools each document bucket.
func PoolEmbeddingWindowBuckets(buckets [][][]float32) ([][]float32, error) {
	out := make([][]float32, len(buckets))
	for i, vectors := range buckets {
		if len(vectors) == 0 {
			return nil, fmt.Errorf("Missing embedding vectors for document %d", i)
		}
		pooled, err := PoolWindowVectors(vectors)
		if err != nil {
			return nil, err
		}
		out[i] = pooled
	}
	return out, nil
}

// EmbedTextsWithBackend prefers EmbedInputs when non-nil results path is available.
func EmbedTextsWithBackend(backend EmbeddingBackend, texts []string) ([][]float32, error) {
	if vectors, err := backend.EmbedInputs(texts); err == nil && vectors != nil {
		return vectors, nil
	}
	return backend.EmbedDocuments(texts)
}

func dequantizeEmbedding(raw json.RawMessage) ([]float32, error) {
	var floats []float64
	if err := json.Unmarshal(raw, &floats); err == nil {
		out := make([]float32, len(floats))
		for i, v := range floats {
			out[i] = float32(v)
		}
		return out, nil
	}
	var iq Int8Embedding
	if err := json.Unmarshal(raw, &iq); err != nil {
		return nil, fmt.Errorf("Embedding API returned unsupported embedding format")
	}
	if iq.Dtype != "i8" || iq.Values == nil {
		return nil, fmt.Errorf("Embedding API returned unsupported embedding format")
	}
	out := make([]float32, len(iq.Values))
	for i, v := range iq.Values {
		out[i] = float32((float64(v) - iq.ZeroPoint) * iq.Scale)
	}
	return out, nil
}

func vectorsFromResponse(data []EmbeddingResponseItem, expected int) ([][]float32, error) {
	byIndex := map[int][]float32{}
	for _, item := range data {
		if item.Index >= 0 && item.Index < expected {
			vec, err := dequantizeEmbedding(item.Embedding)
			if err != nil {
				return nil, err
			}
			byIndex[item.Index] = vec
		}
	}
	if len(byIndex) != expected {
		return nil, fmt.Errorf(
			"Embedding API returned %d vectors for %d inputs (%d unique indices)",
			len(data), expected, len(byIndex),
		)
	}
	out := make([][]float32, expected)
	for i := 0; i < expected; i++ {
		vec, ok := byIndex[i]
		if !ok {
			return nil, fmt.Errorf("Missing embedding vector at index %d", i)
		}
		out[i] = vec
	}
	return out, nil
}

const embeddingAuthErrorMessage = "Not authorized. Check your API key and/or token balance."

// APIError is an HTTP embedding failure.
type APIError struct {
	Status int
	Body   string
	cause  error
}

func (e *APIError) Error() string {
	if e.Status == 401 || e.Status == 403 {
		return embeddingAuthErrorMessage
	}
	body := e.Body
	if len(body) > 500 {
		body = body[:500]
	}
	return fmt.Sprintf("Embedding API error %d: %s", e.Status, body)
}

func (e *APIError) Unwrap() error {
	return e.cause
}

func newAPIError(status int, body string) *APIError {
	err := &APIError{Status: status, Body: body}
	if status == 401 || status == 403 {
		// Preserve HTTP details while marking it recoverable so MCP tools can offer
		// the device-code flow instead of leaving agents to fall back to shell commands.
		err.cause = autherr.New(embeddingAuthErrorMessage, nil)
	}
	return err
}

func isTransientEmbeddingError(err error) bool {
	ae, ok := err.(*APIError)
	if !ok {
		return false
	}
	switch ae.Status {
	case 429, 500, 502, 503, 504:
		return true
	default:
		return false
	}
}

func isPayloadTooLargeError(err error) bool {
	ae, ok := err.(*APIError)
	if ok && ae.Status == 413 {
		return true
	}
	return strings.Contains(err.Error(), "413")
}

func transientEmbedBackoff(attempt int) time.Duration {
	ms := 250 * (1 << attempt)
	if ms > 4000 {
		ms = 4000
	}
	return time.Duration(ms) * time.Millisecond
}

type httpEmbeddingClient struct {
	endpoint   string
	httpClient *http.Client
}

func (c *httpEmbeddingClient) CreateEmbeddings(input []string, model string, dimensions *int) (*EmbeddingResponse, error) {
	apiKey, err := env.ResolveEmbeddingAPIKey()
	if err != nil {
		return nil, err
	}
	body := map[string]any{"model": model, "input": input}
	if dimensions != nil {
		body["dimensions"] = *dimensions
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, c.endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, newAPIError(resp.StatusCode, string(raw))
	}
	var out EmbeddingResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	if out.Data == nil {
		return nil, fmt.Errorf("Embedding API returned invalid payload")
	}
	return &out, nil
}

func createHTTPClient() EmbeddingClient {
	base := ResolveEmbeddingBaseURL()
	return &httpEmbeddingClient{
		endpoint:   base + "/embeddings",
		httpClient: &http.Client{Timeout: 120 * time.Second},
	}
}

// OpenAIEmbeddingBackend is the HTTP OpenAI-compatible backend.
type OpenAIEmbeddingBackend struct {
	model               string
	dimensions          int
	client              EmbeddingClient
	batchSize           int
	maxEmbedChars       int
	requestedDimensions *int
	statsMu             sync.Mutex
	stats               EmbeddingTransportStats
}

// OpenAIOptions configures OpenAIEmbeddingBackend.
type OpenAIOptions struct {
	Model        string
	BatchSize    int
	MaxEmbedChars int
	Dimensions   *int
	Client       EmbeddingClient
}

// NewOpenAIEmbeddingBackend creates a backend.
func NewOpenAIEmbeddingBackend(opts *OpenAIOptions) *OpenAIEmbeddingBackend {
	b := &OpenAIEmbeddingBackend{
		model:         ResolveEmbeddingModel(),
		client:        createHTTPClient(),
		batchSize:     ResolveEmbeddingBatchSize(),
		maxEmbedChars: ResolveMaxEmbedChars(),
	}
	if opts != nil {
		if opts.Model != "" {
			b.model = opts.Model
		}
		if opts.BatchSize > 0 {
			b.batchSize = opts.BatchSize
		}
		if opts.MaxEmbedChars > 0 {
			b.maxEmbedChars = opts.MaxEmbedChars
		}
		if opts.Dimensions != nil {
			b.requestedDimensions = opts.Dimensions
		}
		if opts.Client != nil {
			b.client = opts.Client
		}
	}
	if b.requestedDimensions == nil {
		b.requestedDimensions = ResolveEmbeddingDimensions(b.model)
	}
	return b
}

func (b *OpenAIEmbeddingBackend) Model() string      { return b.model }
func (b *OpenAIEmbeddingBackend) Dimensions() int    { return b.dimensions }

func (b *OpenAIEmbeddingBackend) EmbedInputs(texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	sanitized := make([]string, len(texts))
	for i, t := range texts {
		sanitized[i] = SanitizeEmbeddingInput(t)
	}
	vectors, err := b.embedBatchRawWithRetry(sanitized)
	if err != nil {
		return nil, err
	}
	for _, vec := range vectors {
		if b.dimensions == 0 && len(vec) > 0 {
			b.dimensions = len(vec)
		}
	}
	return vectors, nil
}

func (b *OpenAIEmbeddingBackend) EmbedDocuments(texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	jobs, buckets := BuildEmbeddingWindowJobs(texts, b.maxEmbedChars)
	if len(jobs) == 0 {
		out := make([][]float32, len(texts))
		for i := range out {
			out[i] = []float32{}
		}
		return out, nil
	}
	batches := BatchEmbeddingWindowJobs(jobs, b.batchSize)
	concurrencyN := concurrency.ResolveWorkerConcurrency()
	_, err := concurrency.MapPoolErr(batches, concurrencyN, func(batch []EmbeddingWindowJob, _ int) (struct{}, error) {
		inputs := make([]string, len(batch))
		for i, job := range batch {
			inputs[i] = job.Text
		}
		vectors, err := b.EmbedInputs(inputs)
		if err != nil {
			return struct{}{}, err
		}
		AssignEmbeddingWindowVectors(batch, vectors, buckets)
		return struct{}{}, nil
	})
	if err != nil {
		return nil, err
	}
	out, err := PoolEmbeddingWindowBuckets(buckets)
	if err != nil {
		return nil, err
	}
	for _, vec := range out {
		if b.dimensions == 0 && len(vec) > 0 {
			b.dimensions = len(vec)
		}
	}
	return out, nil
}

func (b *OpenAIEmbeddingBackend) EmbedQuery(text string) ([]float32, error) {
	vecs, err := b.EmbedDocuments([]string{text})
	if err != nil {
		return nil, err
	}
	if len(vecs) == 0 || vecs[0] == nil {
		return nil, fmt.Errorf("OpenAI returned no embedding for query")
	}
	return vecs[0], nil
}

func (b *OpenAIEmbeddingBackend) requestEmbeddings(texts []string) (*EmbeddingResponse, error) {
	var lastErr error
	for attempt := 0; attempt <= maxTransientEmbedRetries; attempt++ {
		started := time.Now()
		resp, err := b.client.CreateEmbeddings(texts, b.model, b.requestedDimensions)
		elapsed := float64(time.Since(started).Milliseconds())
		if err == nil {
			b.statsMu.Lock()
			b.stats.Requests++
			b.stats.InputItems += len(texts)
			for _, t := range texts {
				b.stats.InputChars += len(t)
			}
			b.stats.TotalRttMs += elapsed
			if elapsed > b.stats.MaxRttMs {
				b.stats.MaxRttMs = elapsed
			}
			b.statsMu.Unlock()
			return resp, nil
		}
		lastErr = err
		if isTransientEmbeddingError(err) && attempt < maxTransientEmbedRetries {
			b.statsMu.Lock()
			b.stats.Retries++
			b.statsMu.Unlock()
			time.Sleep(transientEmbedBackoff(attempt))
			continue
		}
		return nil, err
	}
	return nil, lastErr
}

func (b *OpenAIEmbeddingBackend) embedBatchRawWithRetry(texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	resp, err := b.requestEmbeddings(texts)
	if err == nil {
		vectors, err := vectorsFromResponse(resp.Data, len(texts))
		if err != nil {
			return nil, err
		}
		for _, vec := range vectors {
			if b.requestedDimensions != nil && len(vec) != *b.requestedDimensions {
				return nil, fmt.Errorf(
					"Embedding API returned %d dims for model %s, expected %d",
					len(vec), b.model, *b.requestedDimensions,
				)
			}
			if b.dimensions == 0 {
				b.dimensions = len(vec)
			} else if len(vec) != b.dimensions {
				return nil, fmt.Errorf(
					"Inconsistent embedding dimensions in batch: %d vs %d",
					len(vec), b.dimensions,
				)
			}
		}
		return vectors, nil
	}
	if isPayloadTooLargeError(err) && len(texts) > 1 {
		b.statsMu.Lock()
		b.stats.PayloadTooLarge++
		b.stats.Retries++
		b.statsMu.Unlock()
		mid := (len(texts) + 1) / 2
		left, errL := b.embedBatchRawWithRetry(texts[:mid])
		if errL != nil {
			return nil, errL
		}
		right, errR := b.embedBatchRawWithRetry(texts[mid:])
		if errR != nil {
			return nil, errR
		}
		return append(left, right...), nil
	}
	if isPayloadTooLargeError(err) && len(texts) == 1 {
		b.statsMu.Lock()
		b.stats.PayloadTooLarge++
		b.stats.Retries++
		b.statsMu.Unlock()
		text := texts[0]
		if len(text) <= 128 {
			b.statsMu.Lock()
			b.stats.Errors++
			b.statsMu.Unlock()
			return nil, err
		}
		mid := len(text) / 2
		left, errL := b.embedBatchRawWithRetry([]string{text[:mid]})
		if errL != nil {
			return nil, errL
		}
		right, errR := b.embedBatchRawWithRetry([]string{text[mid:]})
		if errR != nil {
			return nil, errR
		}
		parts := make([][]float32, 0, 2)
		if len(left) > 0 {
			parts = append(parts, left[0])
		}
		if len(right) > 0 {
			parts = append(parts, right[0])
		}
		if len(parts) == 0 {
			b.statsMu.Lock()
			b.stats.Errors++
			b.statsMu.Unlock()
			return nil, err
		}
		pooled, poolErr := PoolWindowVectors(parts)
		if poolErr != nil {
			return nil, poolErr
		}
		return [][]float32{pooled}, nil
	}
	b.statsMu.Lock()
	b.stats.Errors++
	b.statsMu.Unlock()
	return nil, err
}

// Stats returns a copy of transport stats.
func (b *OpenAIEmbeddingBackend) Stats() EmbeddingTransportStats {
	b.statsMu.Lock()
	defer b.statsMu.Unlock()
	return b.stats
}

// ResetStats clears transport stats.
func (b *OpenAIEmbeddingBackend) ResetStats() {
	b.statsMu.Lock()
	b.stats = EmbeddingTransportStats{}
	b.statsMu.Unlock()
}

var (
	defaultBackendMu sync.Mutex
	defaultBackend   *OpenAIEmbeddingBackend
)

// GetEmbeddingBackend returns a shared or model-specific backend.
// When MIRU_SAGEMAKER_* is set, uses AWS InvokeEndpoint (TEI-style body).
func GetEmbeddingBackend(model string) *OpenAIEmbeddingBackend {
	if smCfg, err := ResolveSageMakerConfig(); err == nil && smCfg != nil {
		id := SageMakerModelID(smCfg.EndpointName)
		return NewOpenAIEmbeddingBackend(&OpenAIOptions{
			Model:      id,
			Dimensions: ResolveEmbeddingDimensions(id),
			Client:     NewSageMakerClient(*smCfg),
		})
	}
	if model != "" {
		return NewOpenAIEmbeddingBackend(&OpenAIOptions{
			Model:      model,
			Dimensions: ResolveEmbeddingDimensions(model),
		})
	}
	defaultBackendMu.Lock()
	defer defaultBackendMu.Unlock()
	if defaultBackend == nil {
		defaultBackend = NewOpenAIEmbeddingBackend(nil)
	}
	return defaultBackend
}

// MockBackend is a simple in-memory backend for tests.
type MockBackend struct {
	ModelName string
	Dims      int
	QueryFn   func(text string) ([]float32, error)
	DocsFn    func(texts []string) ([][]float32, error)
}

func (m *MockBackend) Model() string   { return m.ModelName }
func (m *MockBackend) Dimensions() int { return m.Dims }

func (m *MockBackend) EmbedDocuments(texts []string) ([][]float32, error) {
	if m.DocsFn != nil {
		return m.DocsFn(texts)
	}
	out := make([][]float32, len(texts))
	for i := range texts {
		out[i] = make([]float32, m.Dims)
		if m.Dims > 0 {
			out[i][0] = 1
		}
	}
	return out, nil
}

func (m *MockBackend) EmbedQuery(text string) ([]float32, error) {
	if m.QueryFn != nil {
		return m.QueryFn(text)
	}
	vecs, err := m.EmbedDocuments([]string{text})
	if err != nil {
		return nil, err
	}
	return vecs[0], nil
}

func (m *MockBackend) EmbedInputs(texts []string) ([][]float32, error) {
	return m.EmbedDocuments(texts)
}
